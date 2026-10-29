// What camera effects there are. The effects themselves are in effects.ts, which is large and
// loaded only when someone picks one.

export const masks = ["shades", "crown", "cat", "hearts", "clown"] as const;
export type Mask = (typeof masks)[number];
export type Effect = { blur: boolean; mask?: Mask };

export const maskGlyph: Record<Mask, string> = { shades: "🕶️", crown: "👑", cat: "🐱", hearts: "❤️", clown: "🔴" };

let supported: boolean | undefined;

// effectsSupported is LiveKit's check for its background processors (WebGL 2, VideoFrame and a
// way to turn frames back into a track), without loading them. iOS Safari 17 and later pass.
export function effectsSupported(): boolean {
  if (supported !== undefined) return supported;
  try {
    supported =
      typeof OffscreenCanvas !== "undefined" &&
      typeof VideoFrame !== "undefined" &&
      typeof createImageBitmap !== "undefined" &&
      ("MediaStreamTrackProcessor" in window || "captureStream" in HTMLCanvasElement.prototype) &&
      !!document.createElement("canvas").getContext("webgl2");
  } catch {
    supported = false;
  }
  return supported;
}
