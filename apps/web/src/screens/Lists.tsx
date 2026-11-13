import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router";
import { api, type Entry, type EntryKind, type HouseholdDetail, type User } from "../api/client";
import { removeEntry, sameDay, setDone, startOfDay, useEntries, useShowDone } from "../entries";
import { useMoney } from "../money";
import { excerpt, firstPhoto } from "../notes";
import { useSession } from "../session";
import { Empty } from "../ui/Empty";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { MarkdownText } from "../ui/Markdown";
import { Swipe, type SwipeAction } from "../ui/Swipe";
import { ThreadAvatar, threadFor } from "../ui/ThreadAvatar";
import { DoneToggle } from "./DoneToggle";
import { EntrySheet } from "./EntrySheet";
import { Shopping } from "./Shopping";
import { Screen } from "./Screen";

const tabs = ["reminder", "event", "note", "shopping"] as const;

type Sheet = { entry?: Entry; kind: EntryKind } | null;

export function Lists() {
  const { t } = useTranslation();
  const { session } = useSession();
  const [params, setParams] = useSearchParams();
  const [sheet, setSheet] = useState<Sheet>(null);
  const navigate = useNavigate();
  if (session.state !== "signedIn" || !session.household) throw new Error("tab without a household");
  const { user: me, household } = session;
  const tab = tabs.find((k) => k === params.get("show")) ?? "reminder";
  const kind = tab === "shopping" ? null : tab;
  const { entries, failed, reload } = useEntries(household.id, kind);
  const linked = params.get("entry");

  // A notification opens /lists?entry=<id>: show that entry's tab and open it.
  useEffect(() => {
    if (!linked) return;
    let live = true;
    api.GET("/v1/entries/{entryId}", { params: { path: { entryId: linked } } }).then((res) => {
      if (!live) return;
      setParams({ show: res.data?.kind ?? "reminder" }, { replace: true });
      if (res.data?.kind === "note" && !res.data.sealed) navigate(`/lists/notes/${res.data.id}`);
      else if (res.data && !res.data.sealed) setSheet({ entry: res.data, kind: res.data.kind });
    }, () => undefined);
    return () => {
      live = false;
    };
  }, [linked, setParams, navigate]);

  // Shopping adds in its own field at the top, so it has no plus. A note opens on its own page.
  const add = kind ? { label: t(`entries.title.${kind}`), onClick: () => (kind === "note" ? navigate("/lists/notes/new") : setSheet({ kind })) } : undefined;
  return (
    <Screen title={t("tabs.lists")} add={add}>
      <div className="segmented" role="tablist" aria-label={t("entries.kind")}>
        {tabs.map((k) => (
          <button key={k} type="button" role="tab" aria-selected={tab === k} onClick={() => setParams({ show: k }, { replace: true })}>
            {k === "shopping" ? t("shopping.tab") : t(`entries.tabs.${k}`)}
          </button>
        ))}
      </div>
      {kind === null && <Shopping householdId={household.id} userId={me.id} />}
      {kind && failed && (
        <p className="callout error" role="alert">
          {t("entries.failed")}
        </p>
      )}
      {kind && entries?.length === 0 && (
        <Empty title={t(`entries.empty.${kind}.title`)}>{t(`entries.empty.${kind}.body`)}</Empty>
      )}
      {entries && entries.length > 0 && tab === "reminder" && <Reminders entries={entries} household={household} me={me} onOpen={(entry) => setSheet({ entry, kind: entry.kind })} onChange={reload} />}
      {entries && entries.length > 0 && tab === "event" && <Agenda entries={entries} me={me} onOpen={(entry) => setSheet({ entry, kind: entry.kind })} onChange={reload} />}
      {entries && entries.length > 0 && tab === "note" && <Notes entries={entries} household={household} me={me} onOpen={(entry) => navigate(`/lists/notes/${entry.id}`)} onChange={reload} />}
      {sheet && (
        <EntrySheet
          household={household}
          me={me}
          entry={sheet.entry}
          kind={sheet.kind}
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

// Who an entry is for or from, as the person's thread avatar.
function avatar(household: HouseholdDetail, userId: string | undefined) {
  const i = household.members.findIndex((m) => m.user.id === userId);
  const m = household.members[i];
  return m ? <ThreadAvatar name={m.user.name} src={m.user.avatarUrl} thread={threadFor(i)} size="sm" /> : undefined;
}

// The same swipe actions on every kind of entry: right for done where it can be done, left for delete where you may.
function useEntryActions(me: User, onChange: () => void) {
  const { t } = useTranslation();
  return (e: Entry): { start?: SwipeAction; end?: SwipeAction } => ({
    start:
      e.kind === "reminder"
        ? { label: t(e.doneAt ? "entries.reopen" : "entries.done"), icon: "check", tone: "done", onAction: async () => (await setDone(e, !e.doneAt)) && onChange() }
        : undefined,
    end:
      e.authorId === me.id || e.visibility === "shared"
        ? { label: t("entries.delete"), icon: "trash", tone: "danger", onAction: async () => (await removeEntry(e, t("entries.deleteConfirm"))) && onChange() }
        : undefined,
  });
}

export function useWhen() {
  const { t, i18n } = useTranslation();
  const time = new Intl.DateTimeFormat(i18n.language, { hour: "numeric", minute: "2-digit" });
  const day = new Intl.DateTimeFormat(i18n.language, { weekday: "long", day: "numeric", month: "long" });
  const short = new Intl.DateTimeFormat(i18n.language, { weekday: "short", day: "numeric", month: "short" });
  const dayName = (d: Date) => {
    const today = startOfDay(new Date());
    const diff = Math.round((startOfDay(d).getTime() - today.getTime()) / 86400000);
    if (diff === 0) return t("entries.today");
    if (diff === 1) return t("entries.tomorrow");
    if (diff === -1) return t("entries.yesterday");
    return day.format(d);
  };
  // "Today, 17:00" for a moment; "Tue 4 Nov" for a day further out in a list.
  const moment = (iso: string) => {
    const d = new Date(iso);
    const diff = Math.abs(startOfDay(d).getTime() - startOfDay(new Date()).getTime()) / 86400000;
    return `${diff <= 1 ? dayName(d) : short.format(d)}, ${time.format(d)}`;
  };
  const span = (e: Entry) => {
    if (e.allDay || !e.startsAt) return t("entries.allDay");
    const start = new Date(e.startsAt);
    if (!e.endsAt) return time.format(start);
    const end = new Date(e.endsAt);
    return sameDay(start, end) ? `${time.format(start)} – ${time.format(end)}` : `${time.format(start)} – ${short.format(end)}, ${time.format(end)}`;
  };
  return { dayName, moment, span, day };
}

function Reminders({ entries, household, me, onOpen, onChange }: {
  entries: Entry[];
  household: HouseholdDetail;
  me: User;
  onOpen: (e: Entry) => void;
  onChange: () => void;
}) {
  const { t } = useTranslation();
  const { moment } = useWhen();
  const money = useMoney();
  const actions = useEntryActions(me, onChange);
  const [showDone, toggleDone] = useShowDone(me.id, "reminders");
  const open = entries.filter((e) => !e.doneAt);
  const done = entries.filter((e) => e.doneAt);
  const subtitle = (e: Entry) =>
    [
      e.dueAt && moment(e.dueAt),
      e.repeat !== "never" && t(`entries.repeats.${e.repeat}`),
      e.bill && money(e.bill.cents),
      e.visibility === "private" ? t("entries.visibility.private") : e.assigneeId ? (e.assigneeId === me.id ? t("entries.forMe") : household.members.find((m) => m.user.id === e.assigneeId)?.user.name) : t("entries.forBoth"),
    ]
      .filter(Boolean)
      .join(" · ");
  const row = (e: Entry) => (
    <Swipe key={e.id} {...actions(e)}>
      <div className="entry-row">
        <button
          type="button"
          className={e.doneAt ? "tick tick-done" : "tick"}
          role="checkbox"
          aria-checked={!!e.doneAt}
          aria-label={e.text}
          onClick={async () => (await setDone(e, !e.doneAt)) && onChange()}
        >
          {e.doneAt && <Icon name="check" size="sm" />}
        </button>
        <ListRow title={<span className={e.doneAt ? "have" : undefined}>{e.text}</span>} subtitle={subtitle(e)} trailing={avatar(household, e.assigneeId)} onClick={() => onOpen(e)} />
      </div>
    </Swipe>
  );
  return (
    <>
      {open.length > 0 && <div className="hl-list">{open.map(row)}</div>}
      {done.length > 0 && (
        <section className="group">
          <div className="shopping-done">
            <h2 className="label group-title">{t("entries.done")}</h2>
            <DoneToggle count={done.length} shown={showDone} onToggle={toggleDone} />
          </div>
          {showDone && <div className="hl-list">{done.map(row)}</div>}
        </section>
      )}
    </>
  );
}

// Events by day, from today on.
function Agenda({ entries, me, onOpen, onChange }: { entries: Entry[]; me: User; onOpen: (e: Entry) => void; onChange: () => void }) {
  const { dayName, span } = useWhen();
  const actions = useEntryActions(me, onChange);
  const days = new Map<number, Entry[]>();
  for (const e of entries) {
    const key = startOfDay(new Date(e.startsAt ?? e.createdAt)).getTime();
    days.set(key, [...(days.get(key) ?? []), e]);
  }
  return [...days].map(([key, es]) => (
    <section className="group" key={key}>
      <h2 className="label group-title">{dayName(new Date(key))}</h2>
      <div className="hl-list">
        {es.map((e) => (
          <Swipe key={e.id} {...actions(e)}>
            <ListRow title={e.text} subtitle={span(e)} trailing={e.visibility === "private" ? <Icon name="lock" size="sm" /> : undefined} onClick={() => onOpen(e)} />
          </Swipe>
        ))}
      </div>
    </section>
  ));
}

function Notes({ entries, household, me, onOpen, onChange }: { entries: Entry[]; household: HouseholdDetail; me: User; onOpen: (e: Entry) => void; onChange: () => void }) {
  const { t } = useTranslation();
  const { day } = useWhen();
  const actions = useEntryActions(me, onChange);
  return entries.map((e) => {
    const author = household.members.find((m) => m.user.id === e.authorId)?.user.name ?? "";
    const chip = (
      <span className={e.visibility === "reveal" ? "hl-chip hl-chip-reveal" : e.visibility === "shared" ? "hl-chip hl-chip-shared" : "hl-chip"}>
        <Icon name={e.visibility === "private" ? "lock" : e.visibility} size="sm" />
        <span>{t(`entries.visibility.${e.visibility}`)}</span>
      </span>
    );
    const eyebrow = (
      <>
        <span>{e.authorId === me.id ? t("entries.byMe") : author}</span>
        {chip}
      </>
    );
    if (e.sealed || (e.authorId !== me.id && e.visibility !== "shared")) {
      return (
        <article className="hl-card hl-card-sunken" key={e.id}>
          <div className="hl-eyebrow">{eyebrow}</div>
          {e.sealed ? (
            <p className="hl-card-body">{t("entries.sealed", { name: author, date: day.format(new Date(e.revealAt ?? e.createdAt)) })}</p>
          ) : (
            <div className="bubble-md note-md">
              <MarkdownText text={e.text ?? ""} photos />
            </div>
          )}
        </article>
      );
    }
    const photo = firstPhoto(e.text ?? "");
    const more = excerpt(e.text ?? "");
    return (
      <Swipe key={e.id} className="swipe-card" {...actions(e)}>
        <button type="button" className="hl-card note" onClick={() => onOpen(e)}>
          <span className="hl-eyebrow">{eyebrow}</span>
          <span className="note-head">
            <span className="note-head-text">
              <span className="headline note-title">{e.title || t("notes.untitled")}</span>
              {more && <span className="note-text">{more}</span>}
            </span>
            {photo && <img className="note-thumb" src={photo} alt="" loading="lazy" />}
          </span>
          {e.visibility === "reveal" && e.revealAt && <span className="caption muted">{t("entries.opens", { date: day.format(new Date(e.revealAt)) })}</span>}
        </button>
      </Swipe>
    );
  });
}
