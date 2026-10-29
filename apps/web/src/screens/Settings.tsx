import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type McpToken, type TokenAccess, type VoiceSettings } from "../api/client";
import { ownAvatar, removeAvatar, square, uploadAvatar } from "../avatar";
import { currentSubscription, pushKey, pushSupport, subscribe, unsubscribe } from "../push";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { ListRow } from "../ui/ListRow";
import { ThreadAvatar, threadFor } from "../ui/ThreadAvatar";
import { Screen } from "./Screen";

export function Settings() {
  const { t } = useTranslation();
  const { session, signOut } = useSession();
  if (session.state !== "signedIn") return null;
  const { user, household } = session;
  return (
    <Screen title={t("settings.title")}>
      <section className="group">
        <h2 className="label group-title">{t("settings.you")}</h2>
        <div className="hl-list">
          <ListRow leading={<ThreadAvatar name={user.name} src={user.avatarUrl} />} title={user.name} subtitle={user.email} />
        </div>
        <Photo url={user.avatarUrl} />
      </section>
      {household && (
        <section className="group">
          <h2 className="label group-title">
            {t("settings.household")} · {household.name}
          </h2>
          <div className="hl-list">
            {household.members.map((m, i) => (
              <ListRow
                key={m.user.id}
                leading={<ThreadAvatar name={m.user.name} src={m.user.avatarUrl} thread={threadFor(i)} />}
                title={m.user.name}
                trailing={t(`settings.${m.role}`)}
              />
            ))}
          </div>
          <Invite householdId={household.id} />
        </section>
      )}
      {household && <CalendarLink householdId={household.id} />}
      {household && <OtherApps householdId={household.id} />}
      <Voice />
      <Notifications />
      <Button variant="danger" block onClick={signOut}>
        {t("auth.signOut")}
      </Button>
    </Screen>
  );
}

// Your own picture: one picked here wins over the one the sign-in sends, until it is removed.
function Photo({ url }: { url?: string }) {
  const { t } = useTranslation();
  const { reload } = useSession();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);

  async function run(work: () => Promise<unknown>) {
    setBusy(true);
    setFailed(false);
    const ok = await work().catch(() => undefined);
    setBusy(false);
    if (ok) reload();
    else setFailed(true);
  }

  return (
    <>
      <div className="avatar-actions">
        <label className="hl-btn hl-btn-secondary hl-btn-sm">
          {t(url ? "settings.photoChange" : "settings.photo")}
          <input
            className="visually-hidden"
            type="file"
            accept="image/*"
            disabled={busy}
            onChange={(e) => {
              const file = e.target.files?.[0];
              e.target.value = "";
              if (file) void run(async () => uploadAvatar(await square(file)));
            }}
          />
        </label>
        {ownAvatar(url) && (
          <Button variant="plain" size="sm" disabled={busy} onClick={() => void run(removeAvatar)}>
            {t("settings.photoRemove")}
          </Button>
        )}
      </div>
      {failed && (
        <p className="callout error" role="alert">
          {t("settings.photoFailed")}
        </p>
      )}
    </>
  );
}

const voices: VoiceSettings["voice"][] = ["marin", "cedar", "coral", "sage", "shimmer", "nova", "alloy", "ash", "ballad", "echo", "fable", "onyx", "verse"];
const tones: VoiceSettings["tone"][] = ["warm", "calm", "playful", "clear"];

// How Homeloom sounds on this person's calls with it. Each change is saved; Listen plays a short
// sentence with the settings as they are.
function Voice() {
  const { t } = useTranslation();
  const [voice, setVoice] = useState<VoiceSettings | null>(null);
  const [playing, setPlaying] = useState(false);
  const [failed, setFailed] = useState(false);
  const saving = useRef<ReturnType<typeof setTimeout>>(undefined);
  const audio = useRef<AudioContext>(undefined);

  useEffect(() => {
    let live = true;
    api.GET("/v1/me/voice").then(
      (res) => live && res.data?.available && setVoice(res.data.settings),
      () => undefined,
    );
    return () => {
      live = false;
      clearTimeout(saving.current);
      void audio.current?.close().catch(() => undefined);
    };
  }, []);

  // The slider changes often, so a change is saved once it settles.
  function change(next: VoiceSettings) {
    setVoice(next);
    setFailed(false);
    clearTimeout(saving.current);
    saving.current = setTimeout(async () => {
      const res = await api.PUT("/v1/me/voice", { body: next }).catch(() => undefined);
      if (!res?.data) setFailed(true);
    }, 400);
  }

  // iOS plays sound only from a tap: the audio context is resumed in it, before the preview loads.
  async function listen(v: VoiceSettings) {
    const ctx = (audio.current ??= new AudioContext());
    void ctx.resume();
    setPlaying(true);
    setFailed(false);
    try {
      const res = await api.POST("/v1/me/voice/preview", { body: v, parseAs: "arrayBuffer" });
      if (!res.data) throw new Error("no preview");
      const source = ctx.createBufferSource();
      source.buffer = await ctx.decodeAudioData(res.data);
      source.connect(ctx.destination);
      source.onended = () => setPlaying(false);
      source.start();
    } catch {
      setPlaying(false);
      setFailed(true);
    }
  }

  if (!voice) return null;
  const name = (v: string) => v.charAt(0).toUpperCase() + v.slice(1);
  return (
    <section className="group">
      <h2 className="label group-title">{t("voice.title")}</h2>
      <div className="hl-card voice">
        <label className="field">
          <span className="label">{t("voice.voice")}</span>
          <select className="input" value={voice.voice} onChange={(e) => change({ ...voice, voice: e.target.value as VoiceSettings["voice"] })}>
            {voices.map((v) => (
              <option key={v} value={v}>
                {name(v)}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span className="label">{t("voice.speed")}</span>
          <input
            type="range"
            className="voice-speed"
            min={0.75}
            max={1.5}
            step={0.05}
            value={voice.speed}
            aria-valuetext={`${voice.speed.toFixed(2)}×`}
            onChange={(e) => change({ ...voice, speed: Number(e.target.value) })}
          />
          <span className="voice-speed-ends caption" aria-hidden="true">
            <span>{t("voice.slower")}</span>
            <span>{t("voice.faster")}</span>
          </span>
        </label>
        <div className="field">
          <span className="label">{t("voice.tone")}</span>
          <div className="segmented" role="tablist" aria-label={t("voice.tone")}>
            {tones.map((tone) => (
              <button key={tone} type="button" role="tab" aria-selected={voice.tone === tone} onClick={() => change({ ...voice, tone })}>
                {t(`voice.tones.${tone}`)}
              </button>
            ))}
          </div>
        </div>
        <div className="hl-card-footer">
          <Button size="sm" variant="secondary" disabled={playing} onClick={() => void listen(voice)}>
            {t("voice.listen")}
          </Button>
        </div>
      </div>
      {failed && (
        <p className="callout error" role="alert">
          {t("voice.failed")}
        </p>
      )}
    </section>
  );
}

// Notifications are per device: this switch is about this phone or browser only.
function Notifications() {
  const { t, i18n } = useTranslation();
  const [support] = useState(pushSupport);
  const [key, setKey] = useState<string | null>(null);
  const [sub, setSub] = useState<PushSubscription | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<"failed" | "blocked" | null>(null);

  // The key is fetched before the tap, so the tap can go straight to iOS's permission prompt.
  useEffect(() => {
    if (support !== "ok") return;
    let live = true;
    Promise.all([pushKey(), currentSubscription()])
      .then(([k, s]) => live && (setKey(k), setSub(s)))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [support]);

  async function toggle() {
    if (!key) return;
    setBusy(true);
    setError(null);
    try {
      if (sub) {
        await unsubscribe(sub);
        setSub(null);
      } else if (await subscribe(key, i18n.language)) {
        setSub(await currentSubscription());
      } else {
        setError(Notification.permission === "denied" ? "blocked" : "failed");
      }
    } catch {
      setError("failed");
    }
    setBusy(false);
  }

  if (support === "needsInstall") {
    return (
      <section className="group">
        <h2 className="label group-title">{t("push.title")}</h2>
        <p className="callout">{t("push.install")}</p>
      </section>
    );
  }
  if (support !== "ok" || !key) return null;
  return (
    <section className="group">
      <h2 className="label group-title">{t("push.title")}</h2>
      <div className="hl-list">
        <button type="button" className="hl-listrow" role="switch" aria-checked={sub !== null} disabled={busy} onClick={toggle}>
          <span className="hl-listrow-main">
            <span className="hl-listrow-title">{t("push.switch")}</span>
            <span className="hl-listrow-sub">{t("push.hint")}</span>
          </span>
          <span className={sub ? "switch switch-on" : "switch"} aria-hidden="true" />
        </button>
      </div>
      {error && (
        <p className="callout error" role="alert">
          {t(`push.${error}`)}
        </p>
      )}
    </section>
  );
}

// A private webcal:// link that puts events and dated reminders into the phone's calendar. The API
// keeps only a hash, so the link is shown once; a new one replaces the old.
function CalendarLink({ householdId }: { householdId: string }) {
  const { t } = useTranslation();
  const [active, setActive] = useState<boolean | null>(null);
  const [url, setUrl] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const path = { params: { path: { householdId } } };

  useEffect(() => {
    let live = true;
    api.GET("/v1/households/{householdId}/calendar-feed", { params: { path: { householdId } } }).then(
      (res) => live && setActive(res.data?.active ?? null),
      () => undefined,
    );
    return () => {
      live = false;
    };
  }, [householdId]);

  async function create() {
    setBusy(true);
    setFailed(false);
    const res = await api.POST("/v1/households/{householdId}/calendar-feed", path).catch(() => undefined);
    setBusy(false);
    if (!res?.data) return setFailed(true);
    setActive(true);
    setUrl(`webcal://${window.location.host}${res.data.path}`);
  }

  async function turnOff() {
    setBusy(true);
    const res = await api.DELETE("/v1/households/{householdId}/calendar-feed", path).catch(() => undefined);
    setBusy(false);
    if (!res?.response.ok) return setFailed(true);
    setActive(false);
    setUrl(null);
  }

  if (active === null) return null;
  return (
    <section className="group">
      <h2 className="label group-title">{t("calendar.title")}</h2>
      <div className="hl-card">
        <p className="hl-card-body">{t(url ? "calendar.ready" : active ? "calendar.on" : "calendar.body")}</p>
        {url && <p className="invite-link caption">{url}</p>}
        <div className="hl-card-footer">
          {url ? (
            <a className="hl-btn hl-btn-primary hl-btn-sm" href={url}>
              {t("calendar.open")}
            </a>
          ) : (
            <Button size="sm" disabled={busy} onClick={create}>
              {t(active ? "calendar.renew" : "calendar.add")}
            </Button>
          )}
          {active && (
            <Button size="sm" variant="secondary" disabled={busy} onClick={turnOff}>
              {t("calendar.off")}
            </Button>
          )}
        </div>
      </div>
      {failed && (
        <p className="callout error" role="alert">
          {t("calendar.failed")}
        </p>
      )}
    </section>
  );
}

// Keys with which another assistant uses Homeloom's tools at /mcp, and passwords with which a
// calendar app syncs events and reminders over CalDAV, or a notes app syncs notes over WebDAV, each
// as this person. The API keeps only a hash, so a new one is shown once.
function OtherApps({ householdId }: { householdId: string }) {
  const { t, i18n } = useTranslation();
  const [tokens, setTokens] = useState<McpToken[] | null>(null);
  const [name, setName] = useState("");
  const [access, setAccess] = useState<TokenAccess>("calendar");
  const [made, setMade] = useState<{ secret: string; access: TokenAccess } | null>(null);
  const [copied, setCopied] = useState<"address" | "key" | null>(null);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const [round, setRound] = useState(0);
  const path = { params: { path: { householdId } } };
  const calendar = made?.access === "calendar";
  const password = made?.access === "calendar" || made?.access === "notes";
  // A calendar app takes the server's name and finds the rest itself.
  const address = calendar ? window.location.host : `${window.location.origin}/${made?.access === "notes" ? "webdav/" : "mcp"}`;

  useEffect(() => {
    let live = true;
    api.GET("/v1/households/{householdId}/mcp-tokens", { params: { path: { householdId } } }).then(
      (res) => live && setTokens(res.data ?? null),
      () => undefined,
    );
    return () => {
      live = false;
    };
  }, [householdId, round]);

  async function create(e: React.SyntheticEvent) {
    e.preventDefault();
    setBusy(true);
    setFailed(false);
    const res = await api.POST("/v1/households/{householdId}/mcp-tokens", { ...path, body: { name: name.trim(), access } }).catch(() => undefined);
    setBusy(false);
    if (!res?.data) return setFailed(true);
    const { secret, ...token } = res.data;
    setTokens((ts) => [token, ...(ts ?? [])]);
    setMade({ secret, access: token.access });
    setCopied(null);
    setName("");
  }

  // The profile download leaves the page as it is, so the new password is listed once it's made.
  function profileSent() {
    // Later, so the form is sent with the name before it is cleared.
    setTimeout(() => {
      setName("");
      setRound((r) => r + 1);
    }, 2000);
  }

  async function remove(id: string) {
    setBusy(true);
    setFailed(false);
    const res = await api.DELETE("/v1/mcp-tokens/{tokenId}", { params: { path: { tokenId: id } } }).catch(() => undefined);
    setBusy(false);
    if (!res?.response.ok) return setFailed(true);
    setTokens((ts) => ts?.filter((x) => x.id !== id) ?? null);
  }

  async function copy(what: "address" | "key", text: string) {
    await navigator.clipboard.writeText(text).then(() => setCopied(what), () => setFailed(true));
  }

  if (tokens === null) return null;
  const date = new Intl.DateTimeFormat(i18n.language, { day: "numeric", month: "long", year: "numeric" });
  const addressLabel = t(calendar ? "apps.server" : "apps.address");
  const secretLabel = t(password ? "apps.password" : "apps.key");
  return (
    <section className="group">
      <h2 className="label group-title">{t("apps.title")}</h2>
      {tokens.length > 0 && (
        <div className="hl-list">
          {tokens.map((x) => (
            <ListRow
              key={x.id}
              title={x.name}
              subtitle={`${t(`apps.for.${x.access}`)} · ${t("apps.made", { date: date.format(new Date(x.createdAt)) })}`}
              trailing={
                <Button size="sm" variant="secondary" disabled={busy} onClick={() => remove(x.id)}>
                  {t("apps.remove")}
                </Button>
              }
            />
          ))}
        </div>
      )}
      <div className="hl-card">
        {made ? (
          <>
            <p className="hl-card-body">{t(calendar ? "apps.calendarOnce" : made.access === "notes" ? "apps.notesOnce" : "apps.once")}</p>
            <p className="label">{addressLabel}</p>
            <p className="invite-link caption">{address}</p>
            {password && (
              <>
                <p className="label">{t("apps.user")}</p>
                <p className="caption">{t("apps.anyUser")}</p>
              </>
            )}
            <p className="label">{secretLabel}</p>
            <p className="invite-link caption">{made.secret}</p>
            <div className="hl-card-footer">
              <Button size="sm" variant="secondary" onClick={() => copy("address", address)}>
                {copied === "address" ? t("apps.copied") : `${t("apps.copy")} · ${addressLabel}`}
              </Button>
              <Button size="sm" onClick={() => copy("key", made.secret)}>
                {copied === "key" ? t("apps.copied") : `${t("apps.copy")} · ${secretLabel}`}
              </Button>
              <Button size="sm" variant="secondary" onClick={() => setMade(null)}>
                {t("apps.done")}
              </Button>
            </div>
          </>
        ) : (
          // For a calendar the form posts itself: the API answers with a profile, which iOS offers to install.
          <form method="post" action={`/v1/households/${householdId}/calendar-profile`} onSubmit={access === "calendar" ? profileSent : create}>
            <div className="segmented" role="tablist" aria-label={t("apps.kind")}>
              {(["calendar", "notes", "tools"] as const).map((a) => (
                <button key={a} type="button" role="tab" aria-selected={access === a} onClick={() => setAccess(a)}>
                  {t(`apps.for.${a}`)}
                </button>
              ))}
            </div>
            <p className="hl-card-body">{t(access === "calendar" ? "apps.calendarBody" : access === "notes" ? "apps.notesBody" : "apps.body")}</p>
            <label className="field">
              <span className="label">{t("apps.name")}</span>
              <input className="input" name="name" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} placeholder={t(access === "tools" ? "apps.namePlaceholder" : "apps.calendarPlaceholder")} required />
            </label>
            <div className="hl-card-footer">
              {access === "calendar" ? (
                <>
                  <Button type="submit" size="sm" disabled={busy || !name.trim()}>
                    {t("apps.addToIPhone")}
                  </Button>
                  <Button type="button" size="sm" variant="secondary" disabled={busy || !name.trim()} onClick={create}>
                    {t("apps.createPassword")}
                  </Button>
                </>
              ) : (
                <Button type="submit" size="sm" disabled={busy || !name.trim()}>
                  {t(access === "notes" ? "apps.createPassword" : "apps.create")}
                </Button>
              )}
            </div>
          </form>
        )}
      </div>
      {failed && (
        <p className="callout error" role="alert">
          {t("apps.failed")}
        </p>
      )}
    </section>
  );
}

// Safari on iOS has the share sheet; desktop browsers often don't.
const canShare = () => typeof navigator.share === "function";

// A one-time link for the other person; the API stores only its hash, so a lost link means a new one.
function Invite({ householdId }: { householdId: string }) {
  const { t, i18n } = useTranslation();
  const [link, setLink] = useState<{ url: string; expires: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const [copied, setCopied] = useState(false);

  async function create() {
    setBusy(true);
    setFailed(false);
    const res = await api.POST("/v1/households/{householdId}/invites", { params: { path: { householdId } } }).catch(() => undefined);
    setBusy(false);
    if (!res?.data) {
      setFailed(true);
      return;
    }
    const expires = new Intl.DateTimeFormat(i18n.language, { weekday: "long", day: "numeric", month: "long" }).format(new Date(res.data.expiresAt));
    setLink({ url: `${window.location.origin}/join/${res.data.code}`, expires });
  }

  async function share(url: string) {
    if (canShare()) {
      // Cancelling the share sheet rejects; there's nothing to do then.
      await navigator.share({ title: "Homeloom", text: t("invite.shareText"), url }).catch(() => undefined);
      return;
    }
    await navigator.clipboard.writeText(url).then(() => setCopied(true), () => setFailed(true));
  }

  if (!link) {
    return (
      <>
        {failed && (
          <p className="callout error" role="alert">
            {t("invite.failed")}
          </p>
        )}
        <Button variant="secondary" block icon="plus" disabled={busy} onClick={create}>
          {t("invite.create")}
        </Button>
      </>
    );
  }
  return (
    <div className="hl-card">
      <p className="hl-card-body">{t("invite.body", { date: link.expires })}</p>
      <p className="invite-link caption">{link.url}</p>
      <div className="hl-card-footer">
        <Button size="sm" onClick={() => share(link.url)}>
          {t(copied ? "invite.copied" : canShare() ? "invite.share" : "invite.copy")}
        </Button>
      </div>
    </div>
  );
}
