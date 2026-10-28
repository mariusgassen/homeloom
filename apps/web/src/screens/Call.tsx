import { Track, type Room } from "livekit-client";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router";
import { useCall, type CallState } from "../call";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { ListRow } from "../ui/ListRow";
import { cx, Icon } from "../ui/Icon";
import { ThreadAvatar, threadFor } from "../ui/ThreadAvatar";

// The others in the household, with their thread colours.
function usePartners() {
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) return { householdId: undefined, partners: [] };
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
        onClick={() => (only ? start(householdId, only.id) : setPicking(true))}
      >
        <Icon name="phone" />
      </button>
      {picking && (
        <div className="sheet-backdrop" onClick={(e) => e.target === e.currentTarget && setPicking(false)}>
          <div className="sheet" role="dialog" aria-modal="true" aria-label={t("call.pick")}>
            <div className="sheet-grabber" aria-hidden="true" />
            <div className="sheet-bar">
              <Button variant="plain" onClick={() => setPicking(false)}>
                {t("entries.cancel")}
              </Button>
              <h2 className="headline sheet-title">{t("call.pick")}</h2>
            </div>
            <div className="hl-list sheet-choices">
              {partners.map((p) => (
                <ListRow
                  key={p.id}
                  leading={<ThreadAvatar name={p.name} src={p.avatarUrl} thread={p.thread} />}
                  title={p.name}
                  trailing={<Icon name="phone" />}
                  onClick={() => {
                    setPicking(false);
                    start(householdId, p.id);
                  }}
                />
              ))}
            </div>
          </div>
        </div>
      )}
    </>
  );
}

// /call?from=<id>, where a call notification leads: answering is a tap, so the call can be heard.
export function IncomingCall() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const { start } = useCall();
  const { householdId, partners } = usePartners();
  const caller = partners.find((p) => p.id === params.get("from")) ?? partners[0];
  if (!householdId || !caller) return null;
  return (
    <div className="call">
      <div className="call-who">
        <ThreadAvatar name={caller.name} src={caller.avatarUrl} thread={caller.thread} size="lg" />
        <h1 className="title">{t("call.incoming", { name: caller.name })}</h1>
      </div>
      <div className="call-controls">
        <Button variant="secondary" onClick={() => navigate("/", { replace: true })}>
          {t("call.decline")}
        </Button>
        <Button
          icon="phone"
          onClick={() => {
            start(householdId, caller.id);
            navigate("/us", { replace: true });
          }}
        >
          {t("call.answer")}
        </Button>
      </div>
    </div>
  );
}

// The call itself, over everything else while it lasts.
export function CallView() {
  const { room, peer, state, since, hangUp } = useCall();
  const { partners } = usePartners();
  if (!room) return null;
  const remote = [...room.remoteParticipants.values()][0];
  const partner = partners.find((p) => p.id === peer);
  const remoteVideo = remote?.getTrackPublication(Track.Source.Camera);
  const localVideo = room.localParticipant.getTrackPublication(Track.Source.Camera);
  const showRemote = remoteVideo?.videoTrack && !remoteVideo.isMuted;
  const showLocal = localVideo?.videoTrack && !localVideo.isMuted;
  return (
    <div className={cx("call", showRemote && "call-video")}>
      {showRemote && <Media track={remoteVideo.videoTrack} className="call-remote" />}
      {showLocal && <Media track={localVideo.videoTrack} className="call-self" />}
      {[...room.remoteParticipants.values()].map((p) => {
        const audio = p.getTrackPublication(Track.Source.Microphone)?.audioTrack;
        return audio && <Media key={p.identity} track={audio} />;
      })}
      <div className="call-who">
        {!showRemote && partner && <ThreadAvatar name={partner.name} src={partner.avatarUrl} thread={partner.thread} size="lg" />}
        <h1 className="title">{remote?.name || partner?.name}</h1>
        <Status state={state} since={since} />
        {!room.canPlaybackAudio && <HearButton room={room} />}
      </div>
      <Controls room={room} ended={isOver(state)} onHangUp={hangUp} />
    </div>
  );
}

const isOver = (s: CallState) => s === "ended" || s === "noAnswer" || s === "failed" || s === "micBlocked";

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
    <p className="callout call-status" role={state === "failed" || state === "micBlocked" ? "alert" : "status"}>
      {t(`call.state.${state}`)}
    </p>
  );
}

// iOS sometimes holds the other one's audio back until a tap.
function HearButton({ room }: { room: Room }) {
  const { t } = useTranslation();
  return (
    <Button size="sm" variant="secondary" onClick={() => void room.startAudio()}>
      {t("call.tapToHear")}
    </Button>
  );
}

function Controls({ room, ended, onHangUp }: { room: Room; ended: boolean; onHangUp: () => void }) {
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
