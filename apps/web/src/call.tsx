import { Room, RoomEvent } from "livekit-client";
import { createContext, use, useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { api, type Call } from "./api/client";

// How a call is going. ringing: you're in and waiting for someone else; the last five end it.
export type CallState = "connecting" | "ringing" | "live" | "reconnecting" | "ended" | "noAnswer" | "failed" | "micBlocked" | "linkUsed";

type Value = {
  room: Room | null;
  // The call's household and id, for adding people; a guest has neither.
  call?: { householdId: string; id: string };
  // Who was called, shown until someone joins.
  invited: string[];
  // Whether this is a voice call with Homeloom.
  homeloom: boolean;
  state: CallState;
  // When someone else joined, for the call's timer.
  since?: number;
  // These must run from a tap: iOS only lets a call be heard when its audio starts in a gesture.
  start: (householdId: string, invite: string[]) => void;
  answer: (householdId: string, callId: string) => void;
  talkToHomeloom: (householdId: string) => void;
  joinAsGuest: (token: string, name: string) => void;
  hangUp: () => void;
};

const CallContext = createContext<Value | null>(null);

// Nobody answering within this long ends the call.
const ringFor = 45_000;
// How long "Call ended" and the like stay up before the call view closes.
const lingerFor = 1_500;

export function CallProvider({ children }: { children: ReactNode }) {
  const [room, setRoom] = useState<Room | null>(null);
  const [call, setCall] = useState<Value["call"]>();
  const [invited, setInvited] = useState<string[]>([]);
  const [homeloom, setHomeloom] = useState(false);
  const [state, setState] = useState<CallState>("connecting");
  const [since, setSince] = useState<number>();
  const [, rerender] = useState(0);
  const current = useRef<Room | null>(null);

  const close = useCallback((r: Room) => {
    if (current.current !== r) return;
    current.current = null;
    setRoom(null);
    setCall(undefined);
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

  // enter connects to the call that ask returns, once per tap.
  const enter = useCallback(
    (ask: () => Promise<Call | "linkUsed" | undefined>, householdId: string | undefined, invite: string[], withHomeloom = false) => {
      if (current.current) return;
      const r = new Room({ adaptiveStream: true, dynacast: true });
      current.current = r;
      setRoom(r);
      setInvited(invite);
      setHomeloom(withHomeloom);
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
          const data = await ask();
          if (!data || data === "linkUsed") return finish(r, data ?? "failed");
          if (householdId) setCall({ householdId, id: data.id });
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

  const start = useCallback(
    (householdId: string, invite: string[]) =>
      enter(
        async () => (await api.POST("/v1/households/{householdId}/calls", { params: { path: { householdId } }, body: { invite } })).data,
        householdId,
        invite,
      ),
    [enter],
  );
  const answer = useCallback(
    (householdId: string, callId: string) =>
      enter(
        async () => (await api.POST("/v1/households/{householdId}/calls/{callId}/join", { params: { path: { householdId, callId } } })).data,
        householdId,
        [],
      ),
    [enter],
  );
  // A call with Homeloom is the caller's own: nobody else can be added, so it has no household call.
  const talkToHomeloom = useCallback(
    (householdId: string) =>
      enter(
        async () => (await api.POST("/v1/households/{householdId}/calls/homeloom", { params: { path: { householdId } } })).data,
        undefined,
        [],
        true,
      ),
    [enter],
  );
  const joinAsGuest = useCallback(
    (token: string, name: string) =>
      enter(
        async () => {
          const res = await api.POST("/v1/guest-calls/{token}", { params: { path: { token } }, body: { name, device: guestDevice(token) } });
          return res.response.status === 409 ? "linkUsed" : res.data;
        },
        undefined,
        [],
      ),
    [enter],
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

  return <CallContext value={{ room, call, invited, homeloom, state, since, start, answer, talkToHomeloom, joinAsGuest, hangUp }}>{children}</CallContext>;
}

// guestDevice is this browser's id for a guest link: the first device to join claims the link, and
// keeping the id lets the guest come back. Without storage it lasts as long as the page.
const devices = new Map<string, string>();
function guestDevice(token: string): string {
  const key = `homeloom.guest.${token}`;
  let id = devices.get(key);
  try {
    id ??= localStorage.getItem(key) ?? undefined;
  } catch {
    // private mode or blocked storage
  }
  id ??= crypto.randomUUID();
  devices.set(key, id);
  try {
    localStorage.setItem(key, id);
  } catch {
    // as above
  }
  return id;
}

export function useCall() {
  const v = use(CallContext);
  if (!v) throw new Error("useCall outside CallProvider");
  return v;
}
