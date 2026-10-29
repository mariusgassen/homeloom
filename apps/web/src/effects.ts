import { BackgroundTransformer, ProcessorWrapper } from "@livekit/track-processors";
import { FaceDetector, FilesetResolver, type Detection } from "@mediapipe/tasks-vision";
import type { LocalVideoTrack } from "livekit-client";
import { maskGlyph, type Effect, type Mask } from "./masks";

// Camera effects for calls, run on the phone itself: a blurred background and a mask that follows
// the face. The others see them too, since the effect is part of the video that's sent.

// MediaPipe's runtime and models are served from Homeloom itself (see vite.config.ts), never a CDN.
const runtime = "/mediapipe";
const segmenterModel = "/mediapipe/selfie_segmenter.tflite";
const faceModel = "/mediapipe/blaze_face_short_range.tflite";
const blurRadius = 12;

// The background is LiveKit's; the mask is drawn over its result.
class Effects extends BackgroundTransformer {
  mask?: Mask;
  private faces?: Promise<void>;
  private detector?: FaceDetector;
  private found: Detection[] = [];
  private frames = 0;
  private stage = document.createElement("canvas");

  constructor(effect: Effect) {
    super({ blurRadius: effect.blur ? blurRadius : undefined, assetPaths: { tasksVisionFileSet: runtime, modelAssetPath: segmenterModel } });
    this.mask = effect.mask;
  }

  async set(effect: Effect) {
    this.mask = effect.mask;
    if (effect.mask) await this.loadFaces();
    await this.update({ ...this.options, blurRadius: effect.blur ? blurRadius : undefined });
  }

  // The face detector loads the first time a mask is picked; until it's ready, frames go out bare.
  private loadFaces() {
    this.faces ??= FilesetResolver.forVisionTasks(runtime)
      .then((files) => FaceDetector.createFromOptions(files, { baseOptions: { modelAssetPath: faceModel, delegate: "GPU" }, runningMode: "VIDEO" }))
      .then(
        (d) => void (this.detector = d),
        () => undefined,
      );
    return this.faces;
  }

  async transform(frame: VideoFrame, controller: TransformStreamDefaultController<VideoFrame>) {
    const out = {
      enqueue: (f: VideoFrame) => {
        controller.enqueue(this.mask ? this.decorate(f) : f);
      },
    } as unknown as TransformStreamDefaultController<VideoFrame>;
    await super.transform(frame, out);
  }

  async destroy() {
    await super.destroy();
    this.detector?.close();
  }

  // decorate draws the mask over a frame and hands back a new one in its place.
  private decorate(frame: VideoFrame): VideoFrame {
    const mask = this.mask;
    const detector = this.detector;
    if (!mask || !detector) return frame;
    const w = frame.displayWidth;
    const h = frame.displayHeight;
    // Faces move slower than frames come; looking every other frame halves the cost.
    if (this.frames++ % 2 === 0) {
      try {
        this.found = detector.detectForVideo(frame, performance.now()).detections;
      } catch {
        this.found = [];
      }
    }
    if (this.stage.width !== w || this.stage.height !== h) {
      this.stage.width = w;
      this.stage.height = h;
    }
    const ctx = this.stage.getContext("2d");
    if (!ctx) return frame;
    ctx.drawImage(frame, 0, 0, w, h);
    for (const face of this.found) drawMask(ctx, mask, face, w, h);
    const next = new VideoFrame(this.stage, { timestamp: frame.timestamp });
    frame.close();
    return next;
  }
}

// drawMask places the mask's emoji on a face, sized by the eyes' distance and tilted with them.
function drawMask(ctx: CanvasRenderingContext2D, mask: Mask, face: Detection, w: number, h: number) {
  const [rightEye, leftEye, nose] = face.keypoints;
  if (!rightEye || !leftEye || !nose) return;
  const ex = (leftEye.x - rightEye.x) * w;
  const ey = (leftEye.y - rightEye.y) * h;
  const eyes = Math.hypot(ex, ey);
  const tilt = Math.atan2(ey, ex);
  const mid = { x: ((rightEye.x + leftEye.x) / 2) * w, y: ((rightEye.y + leftEye.y) / 2) * h };
  const glyph = (text: string, x: number, y: number, size: number) => {
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(tilt);
    ctx.font = `${size}px system-ui, "Apple Color Emoji", "Noto Color Emoji", sans-serif`;
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.fillText(text, 0, 0);
    ctx.restore();
  };
  // Positions along the face: up is away from the nose, across the eyes' tilt.
  const up = (d: number) => ({ x: mid.x + Math.sin(tilt) * d, y: mid.y - Math.cos(tilt) * d });
  switch (mask) {
    case "shades":
      glyph(maskGlyph.shades, mid.x, mid.y, eyes * 2.3);
      break;
    case "crown": {
      const at = up(eyes * 1.7);
      glyph(maskGlyph.crown, at.x, at.y, eyes * 2.2);
      break;
    }
    case "cat": {
      const at = up(eyes * 0.15);
      glyph(maskGlyph.cat, at.x, at.y, eyes * 4.2);
      break;
    }
    case "hearts":
      glyph(maskGlyph.hearts, rightEye.x * w, rightEye.y * h, eyes * 0.75);
      glyph(maskGlyph.hearts, leftEye.x * w, leftEye.y * h, eyes * 0.75);
      break;
    case "clown":
      glyph(maskGlyph.clown, nose.x * w, nose.y * h, eyes * 0.6);
      break;
  }
}

// What each camera track is running, so changing the effect reuses it.
const running = new WeakMap<LocalVideoTrack, { wrapper: ProcessorWrapper<Record<string, unknown>>; effects: Effects }>();

// apply puts an effect on a camera track, or takes it off when there's nothing left to do.
export async function apply(camera: LocalVideoTrack, effect: Effect) {
  const current = running.get(camera);
  if (!effect.blur && !effect.mask) {
    running.delete(camera);
    if (current) await camera.stopProcessor();
    return;
  }
  if (current && camera.getProcessor() === current.wrapper) {
    await current.effects.set(effect);
    return;
  }
  const effects = new Effects(effect);
  const wrapper = new ProcessorWrapper(effects, "homeloom-effects");
  running.set(camera, { wrapper, effects });
  await effects.set(effect);
  await camera.setProcessor(wrapper);
}
