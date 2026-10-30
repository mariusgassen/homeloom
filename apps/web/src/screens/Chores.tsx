import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { api, type Chore, type ChoreCompletion, type ChoreFairness, type HouseholdDetail, type User } from "../api/client";
import { completeChore, dueToday, today, useChores } from "../chores";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Empty } from "../ui/Empty";
import { Segmented } from "../ui/Form";
import { cx, Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { Swipe } from "../ui/Swipe";
import { ThreadAvatar, threadFor } from "../ui/ThreadAvatar";
import { ChoreSheet } from "./ChoreSheet";
import { useWhen } from "./Lists";
import { Screen } from "./Screen";

// Who a chore is for or by, as the person's thread avatar.
export function choreAvatar(household: HouseholdDetail, userId: string | undefined) {
  const i = household.members.findIndex((m) => m.user.id === userId);
  const m = household.members[i];
  return m ? <ThreadAvatar name={m.user.name} src={m.user.avatarUrl} thread={threadFor(i)} size="sm" /> : undefined;
}

// A chore's day in words: today, overdue since a day, or the day it comes.
export function useChoreDay() {
  const { t } = useTranslation();
  const { dayName } = useWhen();
  return (c: Chore) => {
    const now = today();
    if (c.dueOn === now) return t("entries.today");
    const day = dayName(new Date(`${c.dueOn}T00:00`));
    return c.dueOn < now ? t("chores.overdue", { day }) : day;
  };
}

// The chores page, opened from Today: what's on today and later, who did how much, and what was
// done lately, with a thank-you for what the others did.
export function Chores() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) throw new Error("page without a household");
  const { user: me, household } = session;
  const { chores, done, fairness, failed, reload } = useChores(household.id, { history: true });
  const [sheet, setSheet] = useState<Chore | "new" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const when = useChoreDay();
  const name = (id: string | undefined) => (id === me.id ? t("chores.me") : (household.members.find((m) => m.user.id === id)?.user.name ?? ""));

  async function tick(c: Chore) {
    const err = await completeChore(household.id, c);
    setError(err === null ? null : err || t("chores.failed"));
    setStatus(null);
    reload();
  }

  async function remove(c: Chore) {
    if (!window.confirm(t("chores.deleteConfirm"))) return;
    await api.DELETE("/v1/households/{householdId}/chores/{choreId}", { params: { path: { householdId: household.id, choreId: c.id } } }).catch(() => undefined);
    reload();
  }

  async function undo(d: ChoreCompletion) {
    const res = await api.DELETE("/v1/households/{householdId}/chore-completions/{completionId}", { params: { path: { householdId: household.id, completionId: d.id } } }).catch(() => undefined);
    setError(res?.response.ok ? null : (res?.error?.message ?? t("chores.failed")));
    setStatus(null);
    reload();
  }

  async function thank(d: ChoreCompletion) {
    const gratitude = t("chores.thanksLine", { name: name(d.userId), chore: d.title });
    const res = await api
      .POST("/v1/households/{householdId}/chore-completions/{completionId}/thanks", { params: { path: { householdId: household.id, completionId: d.id } }, body: { gratitude } })
      .catch(() => undefined);
    setError(res?.data ? null : (res?.error?.message ?? t("chores.failed")));
    setStatus(res?.data ? t(res.data.gratitude ? "chores.thankedWall" : "chores.thanked", { name: name(d.userId) }) : null);
    reload();
  }

  const subtitle = (c: Chore) =>
    [
      when(c),
      c.repeat === "never" ? t("chores.once") : t(`entries.repeats.${c.repeat}`),
      c.assigneeId ? (c.rotate ? t("chores.turnOf", { name: name(c.assigneeId) }) : name(c.assigneeId)) : t("chores.anyone"),
    ].join(" · ");
  const row = (c: Chore) => (
    <Swipe
      key={c.id}
      start={{ label: t("entries.done"), icon: "check", tone: "done", onAction: () => void tick(c) }}
      end={{ label: t("chores.delete"), icon: "trash", tone: "danger", onAction: () => void remove(c) }}
    >
      <div className="entry-row">
        <button type="button" className="tick" role="checkbox" aria-checked={false} aria-label={c.title} onClick={() => tick(c)} />
        <ListRow title={c.title} subtitle={subtitle(c)} trailing={<Effort points={c.points} />} onClick={() => setSheet(c)} />
      </div>
    </Swipe>
  );

  const now = chores?.filter(dueToday) ?? [];
  const later = chores?.filter((c) => !dueToday(c)) ?? [];
  return (
    <Screen
      title={t("chores.title")}
      back={
        <Button variant="plain" size="sm" className="back" onClick={() => navigate("/")}>
          <span className="back-icon">
            <Icon name="chevron" size="sm" />
          </span>
          {t("tabs.today")}
        </Button>
      }
      add={{ label: t("chores.new"), onClick: () => setSheet("new") }}
    >
      {fairness && household.members.length > 1 && <Fairness fairness={fairness} household={household} me={me} />}
      {(failed || error) && (
        <p className="callout error" role="alert">
          {error ?? t("chores.failed")}
        </p>
      )}
      {status && (
        <p className="callout muted" role="status">
          {status}
        </p>
      )}
      {chores?.length === 0 && (
        <Empty
          title={t("chores.emptyTitle")}
          action={
            <Button size="sm" onClick={() => setSheet("new")}>
              {t("chores.new")}
            </Button>
          }
        >
          {t("chores.emptyBody")}
        </Empty>
      )}
      {now.length > 0 && (
        <section className="group">
          <h2 className="label group-title">{t("chores.today")}</h2>
          <div className="hl-list">{now.map(row)}</div>
        </section>
      )}
      {later.length > 0 && (
        <section className="group">
          <h2 className="label group-title">{t("chores.later")}</h2>
          <div className="hl-list">{later.map(row)}</div>
        </section>
      )}
      {done && done.length > 0 && <History done={done} household={household} me={me} onUndo={undo} onThank={thank} />}
      {sheet && (
        <ChoreSheet
          household={household}
          me={me}
          chore={sheet === "new" ? undefined : sheet}
          onClose={() => setSheet(null)}
          onSaved={() => {
            setSheet(null);
            reload();
          }}
        />
      )}
    </Screen>
  );
}

// A chore's effort as one to three dots.
function Effort({ points }: { points: number }) {
  const { t } = useTranslation();
  return (
    <span className="chore-effort" role="img" aria-label={t(`chores.sizes.${points}`)}>
      {[1, 2, 3].map((p) => (
        <span key={p} className={cx("chore-dot", p <= points && "chore-dot-on")} />
      ))}
    </span>
  );
}

// Who did how much this week or month: points as bars in each person's thread colour.
function Fairness({ fairness, household, me }: { fairness: ChoreFairness; household: HouseholdDetail; me: User }) {
  const { t } = useTranslation();
  const [span, setSpan] = useState<"week" | "month">("week");
  const tally = fairness[span];
  const most = Math.max(1, ...tally.members.map((m) => m.points));
  return (
    <section className="hl-card chore-fairness" aria-label={t("chores.fairness")}>
      <div className="chore-fairness-head">
        <span className="label">{t("chores.fairness")}</span>
        <Segmented label={t("chores.fairness")} value={span} options={(["week", "month"] as const).map((s) => ({ value: s, label: t(`chores.${s}`) }))} onChange={setSpan} />
      </div>
      {tally.members.map((m) => {
        const i = household.members.findIndex((x) => x.user.id === m.userId);
        const member = household.members[i];
        if (!member) return null;
        return (
          <div key={m.userId} className="chore-share">
            <ThreadAvatar name={member.user.name} src={member.user.avatarUrl} thread={threadFor(i)} size="sm" />
            <span className="chore-share-main">
              <span className="chore-share-text">
                <span className="callout">{m.userId === me.id ? t("chores.me") : member.user.name}</span>
                <span className="caption muted">{t("chores.points", { count: m.points })}</span>
              </span>
              <span className="chore-bar">
                <span className={cx("chore-bar-fill", threadFor(i) && `chore-bar-${threadFor(i)}`)} style={{ width: `${(m.points / most) * 100}%` }} />
              </span>
            </span>
          </div>
        );
      })}
    </section>
  );
}

// What was done lately, newest first. Your own latest of a chore can be taken back; what the
// others did can be thanked for.
function History({ done, household, me, onUndo, onThank }: {
  done: ChoreCompletion[];
  household: HouseholdDetail;
  me: User;
  onUndo: (d: ChoreCompletion) => void;
  onThank: (d: ChoreCompletion) => void;
}) {
  const { t } = useTranslation();
  const { dayName } = useWhen();
  const seen = new Set<string>();
  const latest = new Set<string>();
  for (const d of done) {
    if (d.choreId && !seen.has(d.choreId)) latest.add(d.id);
    if (d.choreId) seen.add(d.choreId);
  }
  const thankers = (d: ChoreCompletion) =>
    d.thankedBy
      .map((id) => (id === me.id ? t("chores.me") : household.members.find((m) => m.user.id === id)?.user.name))
      .filter(Boolean)
      .join(", ");
  return (
    <section className="group">
      <h2 className="label group-title">{t("chores.history")}</h2>
      <div className="hl-list">
        {done.map((d) => {
          const mine = d.userId === me.id;
          const thanked = d.thankedBy.includes(me.id);
          const sub = [dayName(new Date(`${d.day}T00:00`)), t("chores.points", { count: d.points }), d.thankedBy.length > 0 && t("chores.thankedBy", { names: thankers(d) })].filter(Boolean).join(" · ");
          const content = (
            <ListRow
              leading={choreAvatar(household, d.userId)}
              title={d.title}
              subtitle={sub}
              trailing={
                !mine && (
                  <button type="button" className={cx("screen-action", "chore-thank", thanked && "chore-thanked")} aria-pressed={thanked} aria-label={t("chores.thank", { name: household.members.find((m) => m.user.id === d.userId)?.user.name ?? "" })} disabled={thanked} onClick={() => onThank(d)}>
                    <Icon name="heart" size="sm" />
                  </button>
                )
              }
            />
          );
          return mine && latest.has(d.id) ? (
            <Swipe key={d.id} end={{ label: t("chores.undo"), icon: "close", tone: "danger", onAction: () => onUndo(d) }}>
              {content}
            </Swipe>
          ) : (
            <div key={d.id}>{content}</div>
          );
        })}
      </div>
    </section>
  );
}
