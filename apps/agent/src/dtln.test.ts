import { AudioFrame } from "@livekit/rtc-node";
import * as ort from "onnxruntime-node";
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { DTLN, fft, sampleRate, type Model, type Models } from "./dtln.js";

// Stand-ins for the two models: the mask keeps every bin, the refinement passes its block on.
function fake(output: (input: Float32Array) => Float32Array): Model {
  return {
    inputNames: ["data", "state"],
    outputNames: ["out", "state_out"],
    run: (async (feeds: Record<string, ort.Tensor>) => {
      const input = feeds.data?.data as Float32Array;
      const out = output(input);
      return { out: new ort.Tensor("float32", out, [1, 1, out.length]), state_out: feeds.state };
    }) as Model["run"],
  };
}
const passthrough: Models = { mask: fake((mag) => new Float32Array(mag.length).fill(1)), refine: fake((block) => Float32Array.from(block)) };

describe("fft", () => {
  it("matches the DFT and inverts", () => {
    const n = 16;
    const x = Float32Array.from({ length: n }, (_, i) => Math.sin(i) + i / n);
    const re = Float32Array.from(x);
    const im = new Float32Array(n);
    fft(re, im, false);
    for (let k = 0; k < n; k++) {
      let r = 0;
      let j = 0;
      x.forEach((v, t) => {
        r += v * Math.cos((-2 * Math.PI * k * t) / n);
        j += v * Math.sin((-2 * Math.PI * k * t) / n);
      });
      expect(re[k]).toBeCloseTo(r, 4);
      expect(im[k]).toBeCloseTo(j, 4);
    }
    fft(re, im, true);
    re.forEach((v, i) => expect(v).toBeCloseTo(x[i] ?? NaN, 5));
  });
});

describe("DTLN", () => {
  it("overlap-adds the refined blocks", async () => {
    const dtln = new DTLN(passthrough);
    const x = Float32Array.from({ length: 128 * 8 }, (_, i) => Math.sin(i / 7) / 8);
    const out: number[] = [];
    for (let t = 0; t < 8; t++) out.push(...(await dtln.step(x.subarray(128 * t, 128 * (t + 1)))));
    // Each output sample is the sum of the four overlapping blocks, 384 samples late.
    for (let i = 384; i < out.length; i++) expect(out[i]).toBeCloseTo(4 * (x[i - 384] ?? NaN), 4);
  });

  it("returns denoised frames of the same shape, about a frame late", async () => {
    const dtln = new DTLN(passthrough);
    const frames = 20;
    const n = sampleRate / 100;
    const out: number[] = [];
    for (let f = 0; f < frames; f++) {
      const data = Int16Array.from({ length: n }, (_, i) => 1000 * Math.sin((f * n + i) / 5));
      const frame = dtln.process(new AudioFrame(data, sampleRate, 1, n));
      expect([frame.sampleRate, frame.channels, frame.samplesPerChannel, frame.data.length]).toEqual([sampleRate, 1, n, n]);
      out.push(...frame.data);
      await new Promise((resolve) => setTimeout(resolve, 5));
    }
    expect(out.slice(0, n).every((s) => s === 0)).toBe(true);
    expect(out.slice(-n).some((s) => s !== 0)).toBe(true);
  });

  it("leaves audio it can't handle alone and passes through after a failure", async () => {
    const stereo = new AudioFrame(new Int16Array(960), 48000, 2, 480);
    expect(new DTLN(passthrough).process(stereo)).toBe(stereo);

    const broken = new DTLN({ ...passthrough, mask: { ...passthrough.mask, run: () => Promise.reject(new Error("boom")) } });
    const n = sampleRate / 100;
    broken.process(new AudioFrame(new Int16Array(n), sampleRate, 1, n));
    await new Promise((resolve) => setTimeout(resolve, 5));
    expect(broken.isEnabled()).toBe(false);
  });
});

describe("onnxruntime-node", () => {
  // Silero loads its own copy first, and a second version fails to load in the same process.
  it("is the version Silero uses", () => {
    const read = (path: string) => JSON.parse(readFileSync(new URL(path, import.meta.url), "utf8"));
    const ours = read("../package.json").dependencies["onnxruntime-node"];
    const silero = read("../node_modules/@livekit/agents-plugin-silero/package.json").dependencies["onnxruntime-node"];
    expect(ours).toBe(silero);
  });
});
