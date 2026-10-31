import { ALL_FORMATS, BlobSource, BufferTarget, Conversion, Input, Mp4OutputFormat, Output, QUALITY_MEDIUM } from "mediabunny";

// Videos are made at most this many pixels on their long side before they are sent.
const videoSize = 1280;

// compressVideo makes a picked video a smaller MP4 (H.264 and AAC) in the browser, with the
// phone's own encoder. It resolves to undefined where that can't be done, e.g. without
// WebCodecs, or when sound or picture would be lost; the video then goes as it is. Loaded only
// when a video is picked.
export async function compressVideo(file: Blob, onProgress: (share: number) => void): Promise<Blob | undefined> {
  if (typeof VideoEncoder === "undefined") return undefined;
  const input = new Input({ source: new BlobSource(file), formats: ALL_FORMATS });
  try {
    const target = new BufferTarget();
    const output = new Output({ format: new Mp4OutputFormat({ fastStart: "in-memory" }), target });
    const conversion = await Conversion.init({
      input,
      output,
      tracks: "primary",
      video: (track) => {
        const long = Math.max(track.displayWidth, track.displayHeight);
        const size = long > videoSize ? (track.displayWidth >= track.displayHeight ? { width: videoSize } : { height: videoSize }) : {};
        return { ...size, codec: "avc", bitrate: QUALITY_MEDIUM, forceTranscode: true };
      },
      // Phones record AAC, which is copied as it is; anything else is encoded to it.
      audio: { codec: "aac" },
      showWarnings: false,
    });
    if (!conversion.isValid || conversion.discardedTracks.length > 0) return undefined;
    conversion.onProgress = (share) => onProgress(share);
    await conversion.execute();
    return target.buffer ? new Blob([target.buffer], { type: "video/mp4" }) : undefined;
  } finally {
    input.dispose();
  }
}
