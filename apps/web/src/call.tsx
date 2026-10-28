import { Room, RoomEvent } from "livekit-client";
import { createContext, use, useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { api } from "./api/client";

// How a call is going. ringing: you're in and waiting for the other one; the last three end it.
export type CallState = "connecting" | "ringing" | "live" | "reconnecting" | "ended" | "noAnswer" | "failed" | "micBlocked";

type Value = {
  room: Room | null;
  state: CallState;
  // When the other one joined, for the call's timer.
  since?: number;
  // start must run from a tap: iOS only lets a call be heard when its audio starts in a gesture.
  start: (householdId: string) => void;
  hangUp: () => void;
};

const CallContext = createContext<Value | null>(null);

// Nobody answering within this long ends the call.
const ringFor = 45_000;
// How long "Call ended" and the like stay up before the call view closes.
const lingerFor = 1_500;

export function CallProvider({ children }: { children: ReactNode }) {
  const [room, setRoom] = useState<Room | null>(null);
  const [state, setState] = useState<CallState>("connecting");
  const [since, setSince] = useState<number>();
  const [, rerender] = useState(0);
  const current = useRef<Room | null>(null);

  const close = useCallback((r: Room) => {
    if (current.current !== r) return;
    current.current = null;
    setRoom(null);
    setSince(undefined);
  }, []);

  // finish leaves the call and shows why for a moment, once per call; hanging up closes the view
  // right away.
  const finished = useRef(new WeakSet<Room>());
  const finish = useCallback(
    (r: Room, why: CallState, linger = true) => {
      if (current.current !== r || finished.current.has(r)) return;
      finished.current.add(r);
      setState(why);
      void r.disconnect();
      if (!linger) close(r);
      else setTimeout(() => close(r), why === "ended" ? lingerFor : lingerFor * 2);
    },
    [close],
  );

  const start = useCallback(
    (householdId: string) => {
      if (current.current) return;
      const r = new Room({ adaptiveStream: true, dynacast: true });
      current.current = r;
      setRoom(r);
      setState("connecting");
      void r.startAudio();

      let joined = false;
      const sync = () => {
        if (current.current !== r) return;
        if (r.remoteParticipants.size > 0) {
          if (!joined) setSince(Date.now());
          joined = true;
          setState("live");
        } else if (joined) {
          finish(r, "ended");
        } else {
          setState("ringing");
        }
      };
      const refresh = () => rerender((n) => n + 1);
      r.on(RoomEvent.ParticipantConnected, sync)
        .on(RoomEvent.ParticipantDisconnected, sync)
        .on(RoomEvent.Reconnecting, () => current.current === r && setState("reconnecting"))
        .on(RoomEvent.Reconnected, sync)
        .on(RoomEvent.Disconnected, () => finish(r, joined ? "ended" : "failed"))
        .on(RoomEvent.TrackSubscribed, refresh)
        .on(RoomEvent.TrackUnsubscribed, refresh)
        .on(RoomEvent.TrackMuted, refresh)
        .on(RoomEvent.TrackUnmuted, refresh)
        .on(RoomEvent.LocalTrackPublished, refresh)
        .on(RoomEvent.LocalTrackUnpublished, refresh)
        .on(RoomEvent.AudioPlaybackStatusChanged, refresh);

      void (async () => {
        try {
          const { data } = await api.POST("/v1/households/{householdId}/call", { params: { path: { householdId } } });
          if (!data) return finish(r, "failed");
          await r.connect(data.url, data.token);
        } catch {
          return finish(r, "failed");
        }
        try {
          await r.localParticipant.setMicrophoneEnabled(true);
        } catch {
          return finish(r, "micBlocked");
        }
        sync();
        setTimeout(() => !joined && finish(r, "noAnswer"), ringFor);
      })();
    },
    [finish],
  );

  const hangUp = useCallback(() => {
    if (current.current) finish(current.current, "ended", false);
  }, [finish]);

  // Leaving the app (closing it, not just switching away) leaves the call.
  useEffect(() => {
    const leave = () => void current.current?.disconnect();
    window.addEventListener("pagehide", leave);
    return () => window.removeEventListener("pagehide", leave);
  }, []);

  return <CallContext value={{ room, state, since, start, hangUp }}>{children}</CallContext>;
}

export function useCall() {
  const v = use(CallContext);
  if (!v) throw new Error("useCall outside CallProvider");
  return v;
}
