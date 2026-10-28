import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { useSession } from "../session";
import { ThreadAvatar, threadFor } from "../ui/ThreadAvatar";

// A tab's page: the household button top right, a large title, then content. back replaces the
// overline on pages below a tab; action is a button left of the household one, such as add.
export function Screen({ title, overline, back, action, children }: { title: string; overline?: string; back?: ReactNode; action?: ReactNode; children: ReactNode }) {
  const { t } = useTranslation();
  const { session } = useSession();
  const members = session.state === "signedIn" ? (session.household?.members ?? []) : [];
  return (
    <div className="screen">
      <header className="screen-header">
        <div className="screen-toolbar">
          {back ?? <span className="caption screen-overline">{overline}</span>}
          {action}
          <Link to="/settings" className="screen-household" aria-label={t("settings.open")}>
            <span className="hl-pair">
              {members.map((m, i) => (
                <ThreadAvatar key={m.user.id} name={m.user.name} src={m.user.avatarUrl} thread={threadFor(i)} size="sm" />
              ))}
            </span>
          </Link>
        </div>
        <h1 className="large-title">{title}</h1>
      </header>
      <main className="screen-main">{children}</main>
    </div>
  );
}
