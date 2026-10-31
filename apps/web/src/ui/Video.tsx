import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "./Icon";

// A video without the browser's controls, as in a feed: tap it to play or pause, the corner
// button for sound, a thin line for how far it is. It plays on repeat and stops once it is out
// of view. With autoplay it starts by itself, muted, whenever most of it is in view; without, it
// waits for a tap and plays with sound. "#t=0.001" makes iOS show the first frame, not black.
// The other corner goes fullscreen, with sound.
export function Video({ src, autoplay }: { src: string; autoplay: boolean }) {
  const { t } = useTranslation();
  const ref = useRef<HTMLVideoElement>(null);
  const [playing, setPlaying] = useState(false);
  const [muted, setMuted] = useState(autoplay);
  const [progress, setProgress] = useState(0);
  // Paused by a tap: autoplay leaves it until it has been out of view.
  const held = useRef(false);
  const box = useRef<HTMLDivElement>(null);

  // React doesn't keep the muted property in step with its prop.
  useEffect(() => {
    if (ref.current) ref.current.muted = muted;
  }, [muted]);

  useEffect(() => {
    const v = ref.current;
    if (!v || typeof IntersectionObserver === "undefined") return;
    const seen = new IntersectionObserver(
      ([e]) => {
        if (e?.isIntersecting) {
          if (autoplay && !held.current) v.play().catch(() => undefined);
        } else {
          held.current = false;
          v.pause();
        }
      },
      { threshold: 0.6 },
    );
    seen.observe(v);
    return () => seen.disconnect();
  }, [autoplay]);

  function toggle() {
    const v = ref.current;
    if (!v) return;
    if (v.paused) {
      held.current = false;
      v.play().catch(() => undefined);
    } else {
      held.current = true;
      v.pause();
    }
  }

  function fullscreen() {
    const v = ref.current;
    if (!v) return;
    held.current = false;
    setMuted(false);
    v.muted = false;
    v.play().catch(() => undefined);
    // The iPhone has no fullscreen for elements, only its own player for a video.
    if (box.current?.requestFullscreen && document.fullscreenEnabled) box.current.requestFullscreen().catch(() => undefined);
    else (v as AppleVideo).webkitEnterFullscreen?.();
  }

  return (
    <div className="hl-video" ref={box}>
      <video
        ref={ref}
        className="hl-video-media"
        src={`${src}#t=0.001`}
        playsInline
        loop
        preload="metadata"
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        onTimeUpdate={(e) => setProgress(e.currentTarget.duration ? e.currentTarget.currentTime / e.currentTarget.duration : 0)}
      />
      <button type="button" className="hl-video-tap" aria-label={t(playing ? "video.pause" : "video.play")} onClick={toggle}>
        {!playing && (
          <span className="hl-video-play">
            <Icon name="play" />
          </span>
        )}
      </button>
      {canFullscreen() && (
        <button type="button" className="hl-video-corner hl-video-full" aria-label={t("video.fullscreen")} onClick={fullscreen}>
          <Icon name="expand" size="sm" />
        </button>
      )}
      <button type="button" className="hl-video-corner hl-video-sound" aria-label={t(muted ? "video.soundOn" : "video.soundOff")} aria-pressed={!muted} onClick={() => setMuted(!muted)}>
        <Icon name={muted ? "soundOff" : "sound"} size="sm" />
      </button>
      <span className="hl-video-progress" style={{ transform: `scaleX(${progress})` }} aria-hidden="true" />
    </div>
  );
}

type AppleVideo = HTMLVideoElement & { webkitEnterFullscreen?: () => void };

function canFullscreen() {
  return (typeof document !== "undefined" && document.fullscreenEnabled) || "webkitEnterFullscreen" in HTMLVideoElement.prototype;
}
