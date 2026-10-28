import { useTranslation } from "react-i18next";
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
        </section>
      )}
      <Button variant="danger" block onClick={signOut}>
        {t("auth.signOut")}
      </Button>
    </Screen>
  );
}
