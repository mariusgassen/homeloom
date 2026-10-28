import { useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
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
      <Button variant="danger" block onClick={signOut}>
        {t("auth.signOut")}
      </Button>
    </Screen>
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
