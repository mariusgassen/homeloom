import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type HouseholdDetail, type Moment, type MomentMonth, type User } from "../api/client";
import { dayOf, today, useAutoplay, useResurfaced, useTimeline } from "../moments";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { Empty } from "../ui/Empty";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { ThreadAvatar, threadFor } from "../ui/ThreadAvatar";
import { Video } from "../ui/Video";

// The Us tab's timeline: what comes back today, today's gratitude lines, then month by month.
export function Timeline({ household, me, month, onOpen }: {
  household: HouseholdDetail;
  me: User;
  month?: string;
  onOpen: (m: Moment) => void;
}) {
  const { t } = useTranslation();
  const { months, failed, earlier, loadEarlier, reload } = useTimeline(household.id, month);
  const [lines, setLines] = useState(0);
  const resurfaced = useResurfaced(household.id, lines);
  const people = usePeople(household, me);
  const day = useDayFormat();
  const current = months?.[0]?.month === today().slice(0, 7);
  const todays = current ? (months?.[0]?.moments.filter((m) => m.kind === "gratitude" && m.day === today()) ?? []) : [];

  return (
    <>
      {resurfaced?.onThisDay.map((m) => (
        <article className="hl-card" key={m.id}>
          <div className="hl-eyebrow">{t("moments.onThisDay", { year: dayOf(m).getFullYear() })}</div>
          {m.photoUrl && <img className="moment-photo" src={m.photoUrl} alt="" loading="lazy" />}
          {m.videoUrl && <MomentVideo src={m.videoUrl} />}
          <p className="note-text">{m.text}</p>
        </article>
      ))}
      {resurfaced?.gratitude && (
        <Card tone="sunken" eyebrow={<span>{t(resurfaced.gratitude.authorId === me.id ? "moments.gratefulMe" : "moments.grateful", { name: people.name(resurfaced.gratitude.authorId) })}</span>}>
          {`${resurfaced.gratitude.text ?? ""} · ${day.format(dayOf(resurfaced.gratitude))}`}
        </Card>
      )}
      {current && (
        <Gratitude
          key={todays.find((m) => m.authorId === me.id)?.updatedAt ?? "new"}
          householdId={household.id}
          me={me}
          lines={todays}
          people={people}
          onSaved={() => {
            reload();
            setLines((n) => n + 1);
          }}
        />
      )}
      {failed && (
        <p className="callout error" role="alert">
          {t("moments.failed")}
        </p>
      )}
      {months?.length === 1 && months[0]?.moments.length === 0 && !earlier && <Empty title={t("moments.empty.title")}>{t("moments.empty.body")}</Empty>}
      {months?.map((mm) => <Month key={mm.month} month={mm} me={me} people={people} onOpen={onOpen} />)}
      {earlier && (
        <Button variant="secondary" block onClick={loadEarlier}>
          {t("moments.earlier")}
        </Button>
      )}
    </>
  );
}

type People = ReturnType<typeof usePeople>;

// Names and thread avatars of the household's members, "You" for the viewer.
function usePeople(household: HouseholdDetail, me: User) {
  const { t } = useTranslation();
  const index = (id: string) => household.members.findIndex((m) => m.user.id === id);
  return {
    name: (id: string) => (id === me.id ? t("moments.you") : (household.members[index(id)]?.user.name ?? "")),
    avatar: (id: string) => {
      const m = household.members[index(id)];
      return m ? <ThreadAvatar name={m.user.name} src={m.user.avatarUrl} thread={threadFor(index(id))} size="sm" /> : undefined;
    },
  };
}

function useDayFormat() {
  const { i18n } = useTranslation();
  return new Intl.DateTimeFormat(i18n.language, { weekday: "short", day: "numeric", month: "short" });
}

// One line a day: yours to write or change, and what the others wrote today.
function Gratitude({ householdId, me, lines, people, onSaved }: {
  householdId: string;
  me: User;
  lines: Moment[];
  people: People;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const mine = lines.find((m) => m.authorId === me.id);
  const [editing, setEditing] = useState(!mine);
  const [text, setText] = useState(mine?.text ?? "");
  const [justMe, setJustMe] = useState(mine?.visibility === "private");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const res = await api
      .POST("/v1/households/{householdId}/moments", {
        params: { path: { householdId } },
        body: { kind: "gratitude", text: text.trim(), visibility: justMe ? "private" : "shared" },
      })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) onSaved();
    else setError(res?.error?.message ?? t("moments.failed"));
  }

  return (
    <article className="hl-card">
      <div className="hl-eyebrow">{t("moments.gratitude.eyebrow")}</div>
      {lines
        .filter((m) => m.authorId !== me.id)
        .map((m) => (
          <p className="gratitude-line" key={m.id}>
            {people.avatar(m.authorId)}
            <span>{m.text}</span>
          </p>
        ))}
      {editing || !mine ? (
        <form className="shopping-add" onSubmit={save}>
          <input
            className="input"
            aria-label={t("moments.gratitude.label")}
            placeholder={t("moments.gratitude.placeholder")}
            value={text}
            maxLength={200}
            onChange={(e) => setText(e.target.value)}
          />
          <button type="button" className="screen-action" aria-label={t("moments.justMe")} aria-pressed={justMe} onClick={() => setJustMe(!justMe)}>
            <Icon name={justMe ? "lock" : "shared"} />
          </button>
          <Button type="submit" size="sm" disabled={busy || !text.trim()}>
            {t("moments.gratitude.save")}
          </Button>
        </form>
      ) : (
        <button type="button" className="gratitude-line gratitude-mine" aria-label={t("moments.gratitude.change")} onClick={() => setEditing(true)}>
          {people.avatar(me.id)}
          <span>{mine.text}</span>
          {mine.visibility === "private" && <Icon name="lock" size="sm" />}
        </button>
      )}
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
    </article>
  );
}

// A month: its name and counts, then its moments, with each run of gratitude lines as one list.
function Month({ month, me, people, onOpen }: { month: MomentMonth; me: User; people: People; onOpen: (m: Moment) => void }) {
  const { t, i18n } = useTranslation();
  const day = useDayFormat();
  const [y, mo] = month.month.split("-").map(Number);
  const title = new Intl.DateTimeFormat(i18n.language, { month: "long", year: "numeric" }).format(new Date(y ?? 0, (mo ?? 1) - 1, 1));
  const moments = month.moments.filter((m) => m.kind === "moment").length;
  const thanks = month.moments.length - moments;
  if (month.moments.length === 0) return null;

  const runs: Moment[][] = [];
  for (const m of month.moments) {
    const last = runs.at(-1);
    if (m.kind === "gratitude" && last?.[0]?.kind === "gratitude") last.push(m);
    else runs.push([m]);
  }
  const counts = [moments > 0 && t("moments.count.moments", { count: moments }), thanks > 0 && t("moments.count.thanks", { count: thanks })].filter(Boolean).join(" · ");
  return (
    <section className="group">
      <div className="month-head">
        <h2 className="label group-title">{title}</h2>
        <span className="caption muted">{counts}</span>
      </div>
      {runs.map((run) =>
        run[0]?.kind === "gratitude" ? (
          <div className="hl-list" key={run[0].id}>
            {run.map((m) => (
              <ListRow
                key={m.id}
                leading={people.avatar(m.authorId)}
                title={m.text}
                subtitle={day.format(dayOf(m))}
                trailing={m.visibility === "private" ? <Icon name="lock" size="sm" /> : undefined}
              />
            ))}
          </div>
        ) : (
          run.map((m) => <MomentCard key={m.id} moment={m} me={me} people={people} onOpen={onOpen} />)
        ),
      )}
    </section>
  );
}

function MomentCard({ moment: m, me, people, onOpen }: { moment: Moment; me: User; people: People; onOpen: (m: Moment) => void }) {
  const { t } = useTranslation();
  const day = useDayFormat();
  const eyebrow = (
    <>
      <span>{`${people.name(m.authorId)} · ${day.format(dayOf(m))}`}</span>
      {m.visibility === "private" && <Icon name="lock" size="sm" />}
    </>
  );
  if (m.sealed) {
    return (
      <article className="hl-card hl-card-sunken">
        <div className="hl-eyebrow">{eyebrow}</div>
        <p className="hl-card-body">{t("entries.sealed", { name: people.name(m.authorId), date: day.format(new Date(m.revealAt ?? m.createdAt)) })}</p>
      </article>
    );
  }
  const photo = m.photoUrl && <img className="moment-photo" src={m.photoUrl} alt="" loading="lazy" />;
  const text = (
    <>
      <span className="hl-eyebrow">{eyebrow}</span>
      <span className="note-text">{m.text}</span>
    </>
  );
  if (m.videoUrl) {
    // The video's controls can't sit inside a button, so only the text opens your moment.
    return (
      <article className="hl-card">
        {photo}
        <MomentVideo src={m.videoUrl} />
        {m.authorId === me.id ? (
          <button type="button" className="note moment-open" onClick={() => onOpen(m)}>
            {text}
          </button>
        ) : (
          text
        )}
      </article>
    );
  }
  return m.authorId === me.id ? (
    <button type="button" className="hl-card note" onClick={() => onOpen(m)}>
      {photo}
      {text}
    </button>
  ) : (
    <article className="hl-card">
      {photo}
      {text}
    </article>
  );
}

// A moment's video, playing by itself or not as set on this device.
function MomentVideo({ src }: { src: string }) {
  const [autoplay] = useAutoplay();
  return <Video src={src} autoplay={autoplay} />;
}
