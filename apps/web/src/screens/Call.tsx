import { Track, type Participant, type Room } from "livekit-client";
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
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
  const { t } = useTranslation();
  const { room, call, invited, homeloom, state, since, hangUp } = useCall();
  const { partners } = usePartners();
  const [adding, setAdding] = useState(false);
  if (!room) return null;
  const others = [...room.remoteParticipants.values()];
  const localVideo = room.localParticipant.getTrackPublication(Track.Source.Camera);
  const showLocal = localVideo?.videoTrack && !localVideo.isMuted;
  const names = others.length
    ? others.map((p) => nameOf(p, t))
    : homeloom
      ? [t("call.homeloom")]
      : invited.map((id) => partners.find((p) => p.id === id)?.name).filter((n) => n !== undefined);
  const waitingFor = others.length === 0 && invited.length === 1 ? partners.find((p) => p.id === invited[0]) : undefined;
  return (
    <div className={cx("call", others.length > 0 && "call-live")}>
      {others.length > 0 && (
        <div className={cx("call-grid", others.length === 1 && "call-grid-one")}>
          {others.map((p) => (
            <Tile key={p.identity} participant={p} person={partners.find((m) => m.id === p.identity)} />
          ))}
        </div>
      )}
      {showLocal && <Media track={localVideo.videoTrack} className="call-self" />}
      <div className="call-who">
        {waitingFor && <ThreadAvatar name={waitingFor.name} src={waitingFor.avatarUrl} thread={waitingFor.thread} size="lg" />}
        <h1 className="title">{names.join(", ")}</h1>
        <Status state={state} since={since} />
        {!room.canPlaybackAudio && <HearButton room={room} />}
      </div>
      <Controls room={room} ended={isOver(state)} onAdd={call ? () => setAdding(true) : undefined} onHangUp={hangUp} />
      {adding && call && <AddSheet room={room} householdId={call.householdId} callId={call.id} people={partners} onClose={() => setAdding(false)} />}
    </div>
  );
}

function nameOf(p: Participant, t: TFunction): string {
  return p.isAgent ? t("call.homeloom") : p.name || t("call.guest.someone");
}

// One person in the call: their video, or their avatar while the camera is off.
function Tile({ participant, person }: { participant: Participant; person?: Person }) {
  const { t } = useTranslation();
  const video = participant.getTrackPublication(Track.Source.Camera);
  const audio = participant.getTrackPublication(Track.Source.Microphone);
  const name = nameOf(participant, t);
  return (
    <div className="call-tile">
      {video?.videoTrack && !video.isMuted ? (
        <Media track={video.videoTrack} className="call-tile-video" />
      ) : (
        participant.isAgent ? <AssistantOrb participant={participant} /> : <ThreadAvatar name={name} src={person?.avatarUrl} thread={person?.thread} size="lg" />
      )}
      {audio?.audioTrack && <Media track={audio.audioTrack} />}
      <span className="caption call-tile-name">
        {name}
        {(!audio || audio.isMuted) && <Icon name="micOff" size="sm" label={t("call.muted")} />}
      </span>
    </div>
  );
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

function Controls({ room, ended, onAdd, onHangUp }: { room: Room; ended: boolean; onAdd?: () => void; onHangUp: () => void }) {
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
        onClick={() => void me.setCameraEnabled(!cam, { facingMode: "user" }).catch(() => undefined)}
      >
        <Icon name={cam ? "video" : "videoOff"} />
      </button>
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
