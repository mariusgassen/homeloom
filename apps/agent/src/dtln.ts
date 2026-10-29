import { AudioFrame, FrameProcessor } from "@livekit/rtc-node";
import * as ort from "onnxruntime-node";
import { join } from "node:path";

// DTLN (Westhausen & Meyer, 2020): a spectral mask model and a time-domain model, both stateful
// LSTMs, run on 32 ms blocks every 8 ms of 16 kHz mono audio.
export const sampleRate = 16000;
const blockLen = 512;
const blockShift = 128;
const bins = blockLen / 2 + 1;
const stateShape = [1, 2, 128, 2];
// Output waiting longer than this is dropped, so a slow moment doesn't leave the call lagging.
const maxBacklog = sampleRate / 10;

export type Model = Pick<ort.InferenceSession, "inputNames" | "outputNames" | "run">;
export type Models = { mask: Model; refine: Model };

// The sessions hold no per-call state, so one pair serves every call in the process.
export async function loadModels(dir: string): Promise<Models> {
  const [mask, refine] = await Promise.all(
    ["model_1.onnx", "model_2.onnx"].map((name) => ort.InferenceSession.create(join(dir, name))),
  );
  return { mask: mask as Model, refine: refine as Model };
}

// Denoises the caller's microphone before it reaches voice detection and speech-to-text. LiveKit
// hands frames over synchronously while ONNX Runtime answers asynchronously, so each frame returns
// the audio denoised so far: about one frame (10 ms) later than it came in.
export class DTLN extends FrameProcessor<AudioFrame> {
  #enabled = true;
  #running = false;
  #input = new Fifo();
  #output = new Fifo();
  #inBlock = new Float32Array(blockLen);
  #outBlock = new Float32Array(blockLen);
  #maskState = zeroState();
  #refineState = zeroState();

  constructor(private readonly models: Models) {
    super();
  }

  isEnabled(): boolean {
    return this.#enabled;
  }

  setEnabled(enabled: boolean): void {
    this.#enabled = enabled;
  }

  process(frame: AudioFrame): AudioFrame {
    if (frame.sampleRate !== sampleRate || frame.channels !== 1) return frame;
    this.#input.push(Float32Array.from(frame.data, (s) => s / 32768));
    void this.#drain();

    const n = frame.samplesPerChannel;
    if (this.#output.length > maxBacklog + n) this.#output.take(this.#output.length - maxBacklog - n);
    const out = this.#output.take(n) ?? new Float32Array(n);
    return new AudioFrame(Int16Array.from(out, (s) => Math.round(Math.max(-1, Math.min(1, s)) * 32767)), sampleRate, 1, n);
  }

  close(): void {
    this.#enabled = false;
    this.#input = new Fifo();
    this.#output = new Fifo();
  }

  // Runs the blocks one at a time, in order, since the models carry state from block to block.
  async #drain(): Promise<void> {
    if (this.#running) return;
    this.#running = true;
    try {
      let block: Float32Array | null;
      while (this.#enabled && (block = this.#input.take(blockShift))) this.#output.push(await this.step(block));
    } catch (err) {
      // Passing the raw audio through beats losing the call.
      console.warn(`noise cancellation failed, passing audio through: ${err}`);
      this.#enabled = false;
    } finally {
      this.#running = false;
    }
  }

  /** Denoises the next 128 samples and returns the next 128 samples of output. */
  async step(samples: Float32Array): Promise<Float32Array> {
    this.#inBlock.copyWithin(0, blockShift);
    this.#inBlock.set(samples, blockLen - blockShift);

    const re = Float32Array.from(this.#inBlock);
    const im = new Float32Array(blockLen);
    fft(re, im, false);
    const mag = new Float32Array(bins);
    for (let i = 0; i < bins; i++) mag[i] = Math.hypot(re[i] ?? 0, im[i] ?? 0);

    const [mask, maskState] = await run(this.models.mask, mag, [1, 1, bins], this.#maskState);
    this.#maskState = maskState;
    // Scale each bin, keeping the spectrum conjugate-symmetric so the inverse is real.
    for (let i = 0; i < blockLen; i++) {
      const m = mask[i < bins ? i : blockLen - i] ?? 0;
      re[i] = (re[i] ?? 0) * m;
      im[i] = (im[i] ?? 0) * m;
    }
    fft(re, im, true);

    const [refined, refineState] = await run(this.models.refine, re, [1, 1, blockLen], this.#refineState);
    this.#refineState = refineState;
    // Overlap-add.
    this.#outBlock.copyWithin(0, blockShift);
    this.#outBlock.fill(0, blockLen - blockShift);
    for (let i = 0; i < blockLen; i++) this.#outBlock[i] = (this.#outBlock[i] ?? 0) + (refined[i] ?? 0);
    return this.#outBlock.slice(0, blockShift);
  }
}

function zeroState(): ort.Tensor {
  return new ort.Tensor("float32", new Float32Array(stateShape.reduce((a, b) => a * b)), stateShape);
}

async function run(model: Model, data: Float32Array, dims: number[], state: ort.Tensor): Promise<[Float32Array, ort.Tensor]> {
  const [dataIn = "", stateIn = ""] = model.inputNames;
  const [dataOut = "", stateOut = ""] = model.outputNames;
  const out = await model.run({ [dataIn]: new ort.Tensor("float32", data, dims), [stateIn]: state });
  const result = out[dataOut];
  const next = out[stateOut];
  if (!result || !next) throw new Error("DTLN model returned unexpected outputs");
  return [result.data as Float32Array, next];
}

/** In-place radix-2 FFT; the inverse is scaled by 1/n. */
export function fft(re: Float32Array, im: Float32Array, inverse: boolean): void {
  const n = re.length;
  for (let i = 1, j = 0; i < n; i++) {
    let bit = n >> 1;
    for (; j & bit; bit >>= 1) j ^= bit;
    j ^= bit;
    if (i < j) {
      [re[i], re[j]] = [re[j] ?? 0, re[i] ?? 0];
      [im[i], im[j]] = [im[j] ?? 0, im[i] ?? 0];
    }
  }
  for (let size = 2; size <= n; size <<= 1) {
    const angle = ((inverse ? 2 : -2) * Math.PI) / size;
    for (let start = 0; start < n; start += size) {
      for (let k = 0; k < size / 2; k++) {
        const a = start + k;
        const b = a + size / 2;
        const wr = Math.cos(angle * k);
        const wi = Math.sin(angle * k);
        const br = re[b] ?? 0;
        const bi = im[b] ?? 0;
        const tr = wr * br - wi * bi;
        const ti = wr * bi + wi * br;
        const ar = re[a] ?? 0;
        const ai = im[a] ?? 0;
        re[a] = ar + tr;
        im[a] = ai + ti;
        re[b] = ar - tr;
        im[b] = ai - ti;
      }
    }
  }
  if (inverse) {
    for (let i = 0; i < n; i++) {
      re[i] = (re[i] ?? 0) / n;
      im[i] = (im[i] ?? 0) / n;
    }
  }
}

/** A queue of samples. */
class Fifo {
  #buf = new Float32Array(4096);
  #start = 0;
  #end = 0;

  get length(): number {
    return this.#end - this.#start;
  }

  push(samples: Float32Array): void {
    if (this.#end + samples.length > this.#buf.length) {
      const buf = new Float32Array(Math.max(this.#buf.length, 2 * (this.length + samples.length)));
      buf.set(this.#buf.subarray(this.#start, this.#end));
      this.#buf = buf;
      this.#end = this.length;
      this.#start = 0;
    }
    this.#buf.set(samples, this.#end);
    this.#end += samples.length;
  }

  /** Removes and returns the next n samples, or null if there aren't that many yet. */
  take(n: number): Float32Array | null {
    if (this.length < n) return null;
    const out = this.#buf.slice(this.#start, this.#start + n);
    this.#start += n;
    return out;
  }
}
