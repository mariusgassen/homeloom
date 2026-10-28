import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
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
      <Notifications />
      <Button variant="danger" block onClick={signOut}>
        {t("auth.signOut")}
      </Button>
    </Screen>
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
