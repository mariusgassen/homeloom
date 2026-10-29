import { createAudioAnalyser, ParticipantEvent, Track, type LocalAudioTrack, type Participant, type RemoteAudioTrack } from "livekit-client";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useCall } from "../call";
import { cx } from "../ui/Icon";
import { Mark } from "../ui/Mark";

// What the voice agent is doing, as LiveKit Agents publishes it on its participant.
type AgentState = "initializing" | "listening" | "thinking" | "speaking";

function agentState(p: Participant): AgentState {
  const s = p.attributes["lk.agent.state"];
  return s === "listening" || s === "thinking" || s === "speaking" ? s : "initializing";
}

// Homeloom on a call: the mark in an orb that listens, weaves while it thinks and pulses with its
// voice. Listening it follows your voice, speaking its own. Without motion only colour and label
// change.
export function AssistantOrb({ participant }: { participant: Participant }) {
  const { t } = useTranslation();
  const { room } = useCall();
  const [state, setState] = useState(() => agentState(participant));
  const [, rerender] = useState(0);
  const orb = useRef<HTMLDivElement>(null);
  const still = useReducedMotion();

  useEffect(() => {
    const changed = () => setState(agentState(participant));
    const tracks = () => rerender((n) => n + 1);
    changed();
    participant.on(ParticipantEvent.AttributesChanged, changed).on(ParticipantEvent.TrackSubscribed, tracks).on(ParticipantEvent.TrackUnsubscribed, tracks);
    return () => {
      participant.off(ParticipantEvent.AttributesChanged, changed).off(ParticipantEvent.TrackSubscribed, tracks).off(ParticipantEvent.TrackUnsubscribed, tracks);
    };
  }, [participant]);

  const me = room?.localParticipant;
  const listening = state === "listening" && me ? me : undefined;
  const speaking = state === "speaking" ? participant : undefined;
  const source = speaking ?? listening;
  const track = source?.getTrackPublication(Track.Source.Microphone)?.audioTrack as LocalAudioTrack | RemoteAudioTrack | undefined;
  const level = useLevel(still ? undefined : track, source);

  // The level goes straight to a CSS variable each frame, so the orb moves without React.
  useEffect(() => {
    const el = orb.current;
    if (!el || still) return;
    let smooth = 0;
    let frame = requestAnimationFrame(function tick() {
      smooth += (level.current() - smooth) * 0.3;
      el.style.setProperty("--level", smooth.toFixed(3));
      frame = requestAnimationFrame(tick);
    });
    return () => {
      cancelAnimationFrame(frame);
      el.style.setProperty("--level", "0");
    };
  }, [level, still]);

  return (
    <div className="orb-wrap">
      <div ref={orb} className={cx("orb", `orb-${state}`)}>
        <span className="orb-halo" aria-hidden="true" />
        <span className="orb-disc">
          <Mark size={72} weaving={state === "thinking"} />
        </span>
      </div>
      <p className="caption orb-state" role="status">
        {t(`call.agent.${state}`)}
      </p>
    </div>
  );
}

// useLevel reads how loud track is, from 0 to 1. Web Audio gives a level each frame; until iOS lets
// its audio context run (after a tap), the coarser level LiveKit reports for who stands in.
function useLevel(track: LocalAudioTrack | RemoteAudioTrack | undefined, who: Participant | undefined) {
  const read = useRef<() => number>(() => 0);
  useEffect(() => {
    if (!track) {
      read.current = () => 0;
      return;
    }
    const fallback = () => Math.min(1, (who?.audioLevel ?? 0) * 2);
    let analyser: ReturnType<typeof createAudioAnalyser>;
    try {
      analyser = createAudioAnalyser(track, { fftSize: 256, smoothingTimeConstant: 0.6, minDecibels: -90, maxDecibels: -30 });
    } catch {
      read.current = fallback;
      return;
    }
    const ctx = analyser.analyser.context as AudioContext;
    const resume = () => void ctx.resume().catch(() => undefined);
    resume();
    document.addEventListener("pointerdown", resume);
    read.current = () => (ctx.state === "running" ? Math.min(1, analyser.calculateVolume() * 1.6) : fallback());
    return () => {
      document.removeEventListener("pointerdown", resume);
      read.current = () => 0;
      void analyser.cleanup();
    };
  }, [track, who]);
  return read;
}

const reducedMotion = "(prefers-reduced-motion: reduce)";

function useReducedMotion(): boolean {
  const [reduce, setReduce] = useState(() => window.matchMedia?.(reducedMotion).matches ?? false);
  useEffect(() => {
    const query = window.matchMedia?.(reducedMotion);
    if (!query) return;
    const change = () => setReduce(query.matches);
    query.addEventListener("change", change);
    return () => query.removeEventListener("change", change);
  }, []);
  return reduce;
}
