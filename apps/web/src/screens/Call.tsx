import { RoomEvent, Track, type LocalVideoTrack, type Participant, type Room } from "livekit-client";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type FormEvent, type PointerEvent, type ReactNode } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { api } from "../api/client";
import { AssistantOrb } from "./AssistantOrb";
import { useCall, type CallState } from "../call";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { cx, Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { ThreadAvatar, threadFor, type Thread } from "../ui/ThreadAvatar";

type Person = { id: string; name: string; avatarUrl?: string; thread?: Thread };

// The others in the household, with their thread colours. A guest has none.
function usePartners(): { householdId?: string; partners: Person[] } {
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) return { partners: [] };
  const { household, user } = session;
  return {
    householdId: household.id,
    partners: household.members.map((m, i) => ({ ...m.user, thread: threadFor(i) })).filter((m) => m.id !== user.id),
  };
}

// The phone button in a screen's header. With one other person it calls them; with more it asks
// who to call first.
export function CallButton() {
  const { t } = useTranslation();
  const { start, room } = useCall();
  const { householdId, partners } = usePartners();
  const [picking, setPicking] = useState(false);
  if (!householdId || partners.length === 0) return null;
  const only = partners.length === 1 ? partners[0] : undefined;
  return (
    <>
      <button
        type="button"
        className="screen-action"
        aria-label={only ? t("call.start", { name: only.name }) : t("call.pick")}
        disabled={!!room}
        onClick={() => (only ? start(householdId, [only.id]) : setPicking(true))}
      >
        <Icon name="phone" />
      </button>
      {picking && <PickSheet people={partners} onClose={() => setPicking(false)} onCall={(ids) => start(householdId, ids)} />}
    </>
  );
}

// Who to call, when there's more than one to choose from.
function PickSheet({ people, onClose, onCall }: { people: Person[]; onClose: () => void; onCall: (ids: string[]) => void }) {
  const { t } = useTranslation();
  const [chosen, setChosen] = useState<string[]>([]);
  return (
    <Sheet title={t("call.pick")} onClose={onClose}>
      <div className="hl-list sheet-choices">
        {people.map((p) => (
          <label key={p.id} className="hl-listrow sheet-choice">
            <ThreadAvatar name={p.name} src={p.avatarUrl} thread={p.thread} />
            <span className="hl-listrow-main">
              <span className="hl-listrow-title">{p.name}</span>
            </span>
            <input
              type="checkbox"
              checked={chosen.includes(p.id)}
              onChange={(e) => setChosen((c) => (e.target.checked ? [...c, p.id] : c.filter((id) => id !== p.id)))}
            />
          </label>
        ))}
      </div>
      <Button
        block
        icon="phone"
        disabled={chosen.length === 0}
        onClick={() => {
          onClose();
          onCall(chosen);
        }}
      >
        {t("call.call")}
      </Button>
    </Sheet>
  );
}

function Sheet({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  const { t } = useTranslation();
  return (
    <div className="sheet-backdrop" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className="sheet" role="dialog" aria-modal="true" aria-label={title}>
        <div className="sheet-grabber" aria-hidden="true" />
        <div className="sheet-bar">
          <Button variant="plain" onClick={onClose}>
            {t("entries.done")}
          </Button>
          <h2 className="headline sheet-title">{title}</h2>
        </div>
        {children}
      </div>
    </div>
  );
}

// /call/<id>?from=<id>, where a call notification leads: answering is a tap, so the call can be heard.
export function IncomingCall() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { callId } = useParams();
  const [params] = useSearchParams();
  const { answer } = useCall();
  const { householdId, partners } = usePartners();
  const caller = partners.find((p) => p.id === params.get("from"));
  if (!householdId || !callId) return null;
  return (
    <div className="call">
      <div className="call-who">
        {caller && <ThreadAvatar name={caller.name} src={caller.avatarUrl} thread={caller.thread} size="lg" />}
        <h1 className="title">{caller ? t("call.incoming", { name: caller.name }) : t("call.incomingSomeone")}</h1>
      </div>
      <div className="call-controls">
        <Button variant="secondary" onClick={() => navigate("/", { replace: true })}>
          {t("call.decline")}
        </Button>
        <Button
          icon="phone"
          onClick={() => {
            answer(householdId, callId);
            navigate("/us", { replace: true });
          }}
        >
          {t("call.answer")}
        </Button>
      </div>
    </div>
  );
}

// /guest/<token>: someone without an account joins a call under the name they give.
export function GuestCall({ token }: { token: string }) {
  const { t } = useTranslation();
  const { joinAsGuest, room, state } = useCall();
  const [invitedBy, setInvitedBy] = useState<string | null>();
  const [name, setName] = useState("");
  useEffect(() => {
    api
      .GET("/v1/guest-calls/{token}", { params: { path: { token } } })
      .then((res) => setInvitedBy(res.data?.invitedBy ?? null))
      .catch(() => setInvitedBy(null));
  }, [token]);
  const join = (e: FormEvent) => {
    e.preventDefault();
    if (name.trim()) joinAsGuest(token, name.trim());
  };
  if (invitedBy === undefined) return null;
  return (
    <main className="center">
      {invitedBy === null ? (
        <p className="callout error" role="alert">
          {t("call.guest.expired")}
        </p>
      ) : (
        <form className="call-guest" onSubmit={join}>
          <h1 className="large-title">{t("call.guest.title", { name: invitedBy })}</h1>
          {!room && (state === "ended" || state === "linkUsed") && (
            <p className="callout" role={state === "linkUsed" ? "alert" : undefined}>
              {t(`call.state.${state}`)}
            </p>
          )}
          <label className="field">
            <span className="label">{t("call.guest.name")}</span>
            <input className="input" value={name} maxLength={50} autoComplete="name" onChange={(e) => setName(e.target.value)} />
          </label>
          <Button type="submit" block icon="phone" disabled={!name.trim() || !!room}>
            {t("call.guest.join")}
          </Button>
        </form>
      )}
      <CallView />
    </main>
  );
}

// The call itself, over everything else while it lasts.
export function CallView() {
  const { room } = useCall();
  return room ? <CallStage room={room} /> : null;
}

type Facing = "user" | "environment";

// One call's view; its layout, camera and reactions start fresh with each call.
function CallStage({ room }: { room: Room }) {
  const { t } = useTranslation();
  const { call, invited, homeloom, state, since, hangUp } = useCall();
  const { partners } = usePartners();
  const [adding, setAdding] = useState(false);
  const [reacting, setReacting] = useState(false);
  const [pinned, setPinned] = useState<string>();
  const [swapped, setSwapped] = useState(false);
  const [facing, setFacing] = useState<Facing>("user");
  const cameras = useCameraCount(room.localParticipant.isCameraEnabled);
  const reactions = useReactions(room);
  const others = [...room.remoteParticipants.values()];
  const me = room.localParticipant;
  const localVideo = me.getTrackPublication(Track.Source.Camera);
  const showLocal = !!localVideo?.videoTrack && !localVideo.isMuted;
  const ended = isOver(state);
  const names = others.length
    ? others.map((p) => nameOf(p, t))
    : homeloom
      ? [t("call.homeloom")]
      : invited.map((id) => partners.find((p) => p.id === id)?.name).filter((n) => n !== undefined);
  const waitingFor = others.length === 0 && invited.length === 1 ? partners.find((p) => p.id === invited[0]) : undefined;
  const personOf = (p: Participant) => partners.find((m) => m.id === p.identity);

  const flip = async () => {
    const next = facing === "user" ? "environment" : "user";
    const track = localVideo?.videoTrack as LocalVideoTrack | undefined;
    // iOS lets only one camera run at a time, so the track restarts on the other one.
    await track
      ?.restartTrack({ facingMode: next })
      .then(() => setFacing(next))
      .catch(() => undefined);
  };
  const self = (
    <Tile
      participant={me}
      self={{ mirror: facing === "user", onFlip: cameras > 1 && !ended ? () => void flip() : undefined }}
    />
  );

  // With one other person, tapping your own picture swaps it with theirs, as on FaceTime.
  const one = others.length === 1 ? others[0] : undefined;
  const swap = !!one && showLocal && swapped;
  const grid = swap ? [me] : others;
  const spotlight = others.length > 1 && others.some((p) => p.identity === pinned) ? pinned : undefined;
  return (
    <div className={cx("call", others.length > 0 && "call-live")}>
      {/* Everyone's audio plays apart from their pictures, so moving a picture doesn't cut it. */}
      {others.map((p) => {
        const audio = p.getTrackPublication(Track.Source.Microphone)?.audioTrack;
        return audio && <Media key={p.identity} track={audio} />;
      })}
      {grid.length > 0 && (
        <div className={cx("call-grid", grid.length === 1 && "call-grid-one")}>
          {swap
            ? self
            : others.map((p) => (
                <Tile
                  key={p.identity}
                  participant={p}
                  person={personOf(p)}
                  pinned={spotlight === p.identity}
                  onTap={others.length > 1 ? () => setPinned((id) => (id === p.identity ? undefined : p.identity)) : undefined}
                />
              ))}
        </div>
      )}
      {showLocal && (
        <Floating label={one && showLocal ? t("call.swap") : undefined} onTap={one && showLocal ? () => setSwapped((s) => !s) : undefined}>
          {swap && one ? <Tile participant={one} person={personOf(one)} small /> : self}
        </Floating>
      )}
      <div className="call-who">
        {waitingFor && <ThreadAvatar name={waitingFor.name} src={waitingFor.avatarUrl} thread={waitingFor.thread} size="lg" />}
        <h1 className="title">{names.join(", ")}</h1>
        <Status state={state} since={since} />
        {!room.canPlaybackAudio && <HearButton room={room} />}
      </div>
      <div className="call-reactions" aria-hidden="true">
        {reactions.shown.map((r) => (
          <span key={r.id} className="call-reaction" style={{ left: `${r.x}%` }}>
            {r.emoji}
          </span>
        ))}
      </div>
      {reacting && !ended && (
        <div className="call-tray" role="group" aria-label={t("call.react")}>
          {reactionEmoji.map((e) => (
            <button key={e} type="button" className="call-tray-emoji" aria-label={e} onClick={() => reactions.send(e)}>
              {e}
            </button>
          ))}
        </div>
      )}
      <Controls
        room={room}
        ended={ended}
        facing={facing}
        reacting={others.length > 0 && !homeloom ? reacting : undefined}
        onReact={() => setReacting((r) => !r)}
        onAdd={call ? () => setAdding(true) : undefined}
        onHangUp={hangUp}
      />
      {adding && call && <AddSheet room={room} householdId={call.householdId} callId={call.id} people={partners} onClose={() => setAdding(false)} />}
    </div>
  );
}

function nameOf(p: Participant, t: TFunction): string {
  return p.isAgent ? t("call.homeloom") : p.name || t("call.guest.someone");
}

// One person in the call: their video, or their avatar while the camera is off. Your own picture
// is mirrored while it shows the front camera, the way a mirror would.
function Tile({
  participant,
  person,
  self,
  small,
  pinned,
  onTap,
}: {
  participant: Participant;
  person?: Person;
  self?: { mirror: boolean; onFlip?: () => void };
  small?: boolean;
  pinned?: boolean;
  onTap?: () => void;
}) {
  const { t } = useTranslation();
  const video = participant.getTrackPublication(Track.Source.Camera);
  const audio = participant.getTrackPublication(Track.Source.Microphone);
  const name = nameOf(participant, t);
  return (
    <div
      className={cx("call-tile", pinned && "call-tile-pinned", self?.mirror && "call-mirror")}
      role={onTap ? "button" : undefined}
      tabIndex={onTap ? 0 : undefined}
      aria-pressed={onTap ? !!pinned : undefined}
      aria-label={onTap ? t("call.spotlight", { name }) : undefined}
      onClick={onTap}
      onKeyDown={onTap ? (e) => (e.key === "Enter" || e.key === " ") && onTap() : undefined}
    >
      {video?.videoTrack && !video.isMuted ? (
        <Media track={video.videoTrack} className="call-tile-video" />
      ) : (
        participant.isAgent ? <AssistantOrb participant={participant} /> : <ThreadAvatar name={name} src={person?.avatarUrl} thread={person?.thread} size={small ? undefined : "lg"} />
      )}
      {!self && !small && (
        <span className="caption call-tile-name">
          {name}
          {(!audio || audio.isMuted) && <Icon name="micOff" size="sm" label={t("call.muted")} />}
        </span>
      )}
      {self?.onFlip && (
        <button type="button" className="call-flip" aria-label={t("call.flip")} onClick={self.onFlip}>
          <Icon name="flip" size="sm" />
        </button>
      )}
    </div>
  );
}

type Corner = "top-left" | "top-right" | "bottom-left" | "bottom-right";

// A small picture over the call that can be dragged anywhere and settles in the nearest corner.
// A tap without a drag is onTap.
function Floating({ label, onTap, children }: { label?: string; onTap?: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  const [corner, setCorner] = useState<Corner>("top-right");
  const [offset, setOffset] = useState<{ x: number; y: number }>();
  const drag = useRef<{ x: number; y: number; moved: boolean }>(undefined);
  const settleFrom = useRef<DOMRect>(undefined);

  // After a drag the picture jumps to its corner, then animates there from where it was let go.
  useLayoutEffect(() => {
    const el = ref.current;
    const from = settleFrom.current;
    if (!el || !from) return;
    settleFrom.current = undefined;
    const to = el.getBoundingClientRect();
    el.style.transition = "none";
    el.style.transform = `translate(${from.left - to.left}px, ${from.top - to.top}px)`;
    void el.offsetWidth;
    el.style.transition = "";
    el.style.transform = "";
  }, [corner, offset]);

  const down = (e: PointerEvent<HTMLDivElement>) => {
    if ((e.target as Element).closest("button")) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    drag.current = { x: e.clientX, y: e.clientY, moved: false };
  };
  const move = (e: PointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d) return;
    const x = e.clientX - d.x;
    const y = e.clientY - d.y;
    if (!d.moved && Math.hypot(x, y) < 6) return;
    d.moved = true;
    setOffset({ x, y });
  };
  const up = () => {
    const d = drag.current;
    drag.current = undefined;
    const el = ref.current;
    if (!d || !el) return;
    if (!d.moved) return onTap?.();
    const r = el.getBoundingClientRect();
    const parent = el.parentElement?.getBoundingClientRect();
    if (parent) {
      const top = r.top + r.height / 2 < parent.top + parent.height / 2;
      const left = r.left + r.width / 2 < parent.left + parent.width / 2;
      setCorner(`${top ? "top" : "bottom"}-${left ? "left" : "right"}`);
    }
    settleFrom.current = r;
    setOffset(undefined);
  };
  return (
    <div
      ref={ref}
      className={cx("call-float", `call-float-${corner}`, offset && "call-float-dragging")}
      style={offset && { transform: `translate(${offset.x}px, ${offset.y}px)` }}
      role={onTap ? "button" : undefined}
      tabIndex={onTap ? 0 : undefined}
      aria-label={label}
      onKeyDown={onTap ? (e) => (e.key === "Enter" || e.key === " ") && onTap() : undefined}
      onPointerDown={down}
      onPointerMove={move}
      onPointerUp={up}
      onPointerCancel={() => {
        drag.current = undefined;
        setOffset(undefined);
      }}
    >
      {children}
    </div>
  );
}

// How many cameras this device has, known once one is on (browsers hide them before that).
function useCameraCount(on: boolean): number {
  const [count, setCount] = useState(0);
  useEffect(() => {
    if (!on || !navigator.mediaDevices?.enumerateDevices) return;
    void navigator.mediaDevices
      .enumerateDevices()
      .then((ds) => setCount(ds.filter((d) => d.kind === "videoinput").length))
      .catch(() => undefined);
  }, [on]);
  return count;
}

// The reactions anyone can send; others are ignored, so a guest can't put arbitrary text on screen.
const reactionEmoji = ["❤️", "😂", "😮", "👏", "🎉", "👍"];
const reactionTopic = "reaction";
// How long a reaction floats; matches call-rise in app.css.
const reactionFor = 2_600;

type Reaction = { id: number; emoji: string; x: number };
let reactionId = 0;

// Reactions float up on everyone's screen; they go over the call's data channel, not the server.
function useReactions(room: Room) {
  const [shown, setShown] = useState<Reaction[]>([]);
  const show = useCallback((emoji: string) => {
    const r = { id: ++reactionId, emoji, x: 10 + Math.random() * 75 };
    setShown((s) => [...s.slice(-24), r]);
    setTimeout(() => setShown((s) => s.filter((x) => x !== r)), reactionFor);
  }, []);
  useEffect(() => {
    const received = (payload: Uint8Array, _from?: unknown, _kind?: unknown, topic?: string) => {
      if (topic !== reactionTopic) return;
      const emoji = new TextDecoder().decode(payload);
      if (reactionEmoji.includes(emoji)) show(emoji);
    };
    room.on(RoomEvent.DataReceived, received);
    return () => void room.off(RoomEvent.DataReceived, received);
  }, [room, show]);
  const send = (emoji: string) => {
    show(emoji);
    void room.localParticipant.publishData(new TextEncoder().encode(emoji), { reliable: true, topic: reactionTopic }).catch(() => undefined);
  };
  return { shown, send };
}

// Adds people during a call: members are rung, anyone else gets a guest link.
function AddSheet({ room, householdId, callId, people, onClose }: { room: Room; householdId: string; callId: string; people: Person[]; onClose: () => void }) {
  const { t } = useTranslation();
  const [rung, setRung] = useState<string[]>([]);
  const [link, setLink] = useState<string>();
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);
  const away = people.filter((p) => !room.remoteParticipants.has(p.id));
  const ring = async (id: string) => {
    setRung((r) => [...r, id]);
    const res = await api.POST("/v1/households/{householdId}/calls/{callId}/invites", { params: { path: { householdId, callId } }, body: { invite: [id] } }).catch(() => undefined);
    if (!res?.response.ok) {
      setRung((r) => r.filter((x) => x !== id));
      setFailed(true);
    }
  };
  const makeLink = async () => {
    const res = await api.POST("/v1/households/{householdId}/calls/{callId}/guest-links", { params: { path: { householdId, callId } } }).catch(() => undefined);
    if (res?.data) setLink(`${window.location.origin}/guest/${res.data.token}`);
    else setFailed(true);
  };
  // Sharing needs a fresh tap on iOS, so the link is made first and shared on a second tap.
  const share = async (url: string) => {
    if (navigator.share) {
      await navigator.share({ title: t("call.guest.shareTitle"), url }).catch(() => undefined);
      return;
    }
    await navigator.clipboard?.writeText(url).catch(() => undefined);
    setCopied(true);
  };
  return (
    <Sheet title={t("call.add")} onClose={onClose}>
      {away.length > 0 && (
        <div className="hl-list">
          {away.map((p) => (
            <ListRow
              key={p.id}
              leading={<ThreadAvatar name={p.name} src={p.avatarUrl} thread={p.thread} />}
              title={p.name}
              trailing={rung.includes(p.id) ? t("call.ringing") : <Icon name="phone" />}
              onClick={rung.includes(p.id) ? undefined : () => void ring(p.id)}
            />
          ))}
        </div>
      )}
      <section className="group">
        <h3 className="label group-title">{t("call.guest.heading")}</h3>
        {link ? (
          <>
            <p className="callout call-link">{link}</p>
            <Button block onClick={() => void share(link)}>
              {copied ? t("call.guest.copied") : t("call.guest.share")}
            </Button>
          </>
        ) : (
          <Button block variant="secondary" onClick={() => void makeLink()}>
            {t("call.guest.create")}
          </Button>
        )}
        <p className="caption">{t("call.guest.hint")}</p>
      </section>
      {failed && (
        <p className="callout error" role="alert">
          {t("call.state.failed")}
        </p>
      )}
    </Sheet>
  );
}

const isOver = (s: CallState) => s === "ended" || s === "noAnswer" || s === "failed" || s === "micBlocked" || s === "linkUsed";

function Status({ state, since }: { state: CallState; since?: number }) {
  const { t } = useTranslation();
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (state !== "live") return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [state]);
  if (state === "live" && since) {
    const s = Math.max(0, Math.floor((now - since) / 1000));
    return <p className="callout call-status">{`${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`}</p>;
  }
  return (
    <p className="callout call-status" role={state === "failed" || state === "micBlocked" || state === "linkUsed" ? "alert" : "status"}>
      {t(`call.state.${state}`)}
    </p>
  );
}

// iOS sometimes holds the others' audio back until a tap.
function HearButton({ room }: { room: Room }) {
  const { t } = useTranslation();
  return (
    <Button size="sm" variant="secondary" onClick={() => void room.startAudio()}>
      {t("call.tapToHear")}
    </Button>
  );
}

function Controls({
  room,
  ended,
  facing,
  reacting,
  onReact,
  onAdd,
  onHangUp,
}: {
  room: Room;
  ended: boolean;
  facing: Facing;
  // Whether the reactions are open; undefined when there's nobody to react to.
  reacting?: boolean;
  onReact: () => void;
  onAdd?: () => void;
  onHangUp: () => void;
}) {
  const { t } = useTranslation();
  const me = room.localParticipant;
  const mic = me.isMicrophoneEnabled;
  const cam = me.isCameraEnabled;
  return (
    <div className="call-controls">
      <button
        type="button"
        className="call-control"
        aria-pressed={!mic}
        aria-label={t("call.mute")}
        disabled={ended}
        onClick={() => void me.setMicrophoneEnabled(!mic).catch(() => undefined)}
      >
        <Icon name={mic ? "mic" : "micOff"} />
      </button>
      <button
        type="button"
        className="call-control"
        aria-pressed={cam}
        aria-label={t("call.camera")}
        disabled={ended}
        onClick={() => void me.setCameraEnabled(!cam, { facingMode: facing }).catch(() => undefined)}
      >
        <Icon name={cam ? "video" : "videoOff"} />
      </button>
      {reacting !== undefined && (
        <button type="button" className="call-control" aria-pressed={reacting} aria-label={t("call.react")} disabled={ended} onClick={onReact}>
          <Icon name="smile" />
        </button>
      )}
      {onAdd && (
        <button type="button" className="call-control" aria-label={t("call.add")} disabled={ended} onClick={onAdd}>
          <Icon name="addPerson" />
        </button>
      )}
      <button type="button" className="call-control call-hangup" aria-label={t("call.hangUp")} onClick={onHangUp}>
        <Icon name="phone" />
      </button>
    </div>
  );
}

// Plays a track: video in place, audio out of sight.
function Media({ track, className }: { track: Track; className?: string }) {
  const ref = useRef<HTMLVideoElement & HTMLAudioElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    track.attach(el);
    return () => void track.detach(el);
  }, [track]);
  return track.kind === Track.Kind.Video ? <video ref={ref} className={className} autoPlay playsInline muted /> : <audio ref={ref} autoPlay />;
}
