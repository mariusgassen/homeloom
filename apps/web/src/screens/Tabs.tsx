import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router";
import type { Entry, HouseholdDetail, Moment, Question, User } from "../api/client";
import { dueToday, useEntries } from "../entries";
import { dayOf, useResurfaced } from "../moments";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { Empty } from "../ui/Empty";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { CallButton } from "./Call";
import { EntrySheet } from "./EntrySheet";
import { useWhen } from "./Lists";
import { MomentSheet } from "./MomentSheet";
import { AskForm, QuestionCard, useQuestions } from "./questions";
import { Screen } from "./Screen";
import { Timeline } from "./Timeline";

function useHome() {
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) throw new Error("tab without a household");
  return { me: session.user, household: session.household };
}

// Today shows the question still waiting for you, or else the newest one.
function TodayQuestion({ household, me }: { household: HouseholdDetail; me: User }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { questions, replace } = useQuestions(household.id);
  if (!questions) return null;
  const open = questions.find((q) => !q.revealed && !q.answers.some((a) => a.userId === me.id));
  const q = open ?? questions[0];
  if (!q) {
    return (
      <Card
        eyebrow={<span>{t("today.question.eyebrow")}</span>}
        title={t("today.question.title")}
        footer={
          <Button size="sm" variant="secondary" onClick={() => navigate("/us?show=questions")}>
            {t("questions.askSend")}
          </Button>
        }
      >
        {t("today.question.body")}
      </Card>
    );
  }
  return <QuestionCard question={q} household={household} me={me} eyebrow={t("today.question.eyebrow")} onChange={replace} />;
}

export function Today() {
  const { t, i18n } = useTranslation();
  const { household, me } = useHome();
  const [adding, setAdding] = useState(false);
  const [generation, setGeneration] = useState(0);
  const date = new Intl.DateTimeFormat(i18n.language, { weekday: "long", day: "numeric", month: "long" }).format(new Date());
  const add = (
    <button type="button" className="screen-action screen-add" aria-label={t("entries.add")} onClick={() => setAdding(true)}>
      <Icon name="plus" />
    </button>
  );
  return (
    <Screen title={t("tabs.today")} overline={date} action={add}>
      <Greeting household={household} me={me} />
      <NextUp key={generation} household={household} />
      <TodayQuestion household={household} me={me} />
      <Dinner />
      <TodayMoment household={household} me={me} />
      {adding && (
        <EntrySheet
          household={household}
          me={me}
          onClose={() => setAdding(false)}
          onSaved={() => {
            setAdding(false);
            setGeneration((g) => g + 1);
          }}
        />
      )}
    </Screen>
  );
}

// The part of the day, for the greeting.
function partOfDay(hour: number) {
  if (hour < 5) return "night";
  if (hour < 12) return "morning";
  if (hour < 18) return "afternoon";
  return hour < 22 ? "evening" : "night";
}

// A warm hello to the two of you, rather than a heading over a list.
function Greeting({ household, me }: { household: HouseholdDetail; me: User }) {
  const { t } = useTranslation();
  const first = me.name.trim().split(/\s+/)[0] ?? me.name;
  const partner = household.members.find((m) => m.user.id !== me.id)?.user;
  return (
    <section className="today-hello">
      <h2 className="title">{t(`today.hello.${partOfDay(new Date().getHours())}`, { name: first })}</h2>
      {partner && <p className="callout muted">{t("today.hello.together", { name: partner.name })}</p>}
    </section>
  );
}

// When an entry happens today, for ordering: events by their start, reminders by when they're due.
const happensAt = (e: Entry) => new Date((e.kind === "event" ? e.startsAt : e.dueAt) ?? 0).getTime();

// The next thing today, large, and the rest of the day small beneath it. Each opens in Lists.
function NextUp({ household }: { household: HouseholdDetail }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { entries } = useEntries(household.id);
  const { span, moment } = useWhen();
  if (!entries) return null;
  const now = new Date();
  const [next, ...rest] = entries.filter((e) => dueToday(e, now)).sort((a, b) => happensAt(a) - happensAt(b));
  if (!next) {
    return (
      <div className="today-clear">
        <Icon name="check" />
        <span className="callout">{t("today.next.clear")}</span>
      </div>
    );
  }
  const when = (e: Entry) => (e.kind === "event" ? span(e) : e.dueAt && moment(e.dueAt));
  const open = (e: Entry) => navigate(`/lists?entry=${e.id}`);
  const shown = rest.slice(0, 3);
  return (
    <section className="today-next" aria-label={t("today.next.eyebrow")}>
      <button type="button" className="today-next-main" onClick={() => open(next)}>
        <span className="today-next-icon">
          <Icon name={next.kind === "event" ? "calendar" : "bell"} />
        </span>
        <span className="today-next-text">
          <span className="label today-next-eyebrow">{t("today.next.eyebrow")}</span>
          <span className="headline">{next.text}</span>
          <span className="caption">{when(next)}</span>
        </span>
      </button>
      {shown.length > 0 && (
        <ul className="today-later">
          {shown.map((e) => (
            <li key={e.id}>
              <button type="button" onClick={() => open(e)}>
                <span className="today-later-text">{e.text}</span>
                <span className="caption">{when(e)}</span>
              </button>
            </li>
          ))}
          {rest.length > shown.length && (
            <li>
              <button type="button" className="today-later-more" onClick={() => navigate("/lists")}>
                {t("today.next.more", { count: rest.length - shown.length })}
              </button>
            </li>
          )}
        </ul>
      )}
    </section>
  );
}

// Dinner is an invitation to pick something together, one tap from ideas.
function Dinner() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  return (
    <button type="button" className="today-tile today-dinner" onClick={() => navigate("/cook")}>
      <span className="today-tile-icon">
        <Icon name="cook" />
      </span>
      <span className="today-tile-text">
        <span className="label">{t("today.dinner.eyebrow")}</span>
        <span className="headline">{t("today.dinner.title")}</span>
      </span>
      <Icon name="chevron" size="sm" />
    </button>
  );
}

// Something shared from your story: this day in an earlier year, a thank-you from a while ago,
// or else a nudge to write today's.
function TodayMoment({ household, me }: { household: HouseholdDetail; me: User }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const resurfaced = useResurfaced(household.id, 0);
  const memory = resurfaced?.onThisDay.find((m) => !m.sealed);
  const thanks = resurfaced?.gratitude;
  const author = (id: string) => household.members.find((m) => m.user.id === id)?.user.name ?? "";
  if (memory) {
    return (
      <button type="button" className="today-memory" onClick={() => navigate("/us")}>
        {memory.photoUrl && <img src={memory.photoUrl} alt="" loading="lazy" />}
        <span className="today-memory-text">
          <span className="label">{t("moments.onThisDay", { year: dayOf(memory).getFullYear() })}</span>
          {memory.text && <span className="body">{memory.text}</span>}
        </span>
      </button>
    );
  }
  if (thanks?.text) {
    return (
      <button type="button" className="today-tile today-thanks" onClick={() => navigate("/us")}>
        <span className="today-tile-icon">
          <Icon name="heart" />
        </span>
        <span className="today-tile-text">
          <span className="label">{t(thanks.authorId === me.id ? "moments.gratefulMe" : "moments.grateful", { name: author(thanks.authorId) })}</span>
          <span className="body">{thanks.text}</span>
        </span>
      </button>
    );
  }
  return (
    <button type="button" className="today-tile today-thanks" onClick={() => navigate("/us")}>
      <span className="today-tile-icon">
        <Icon name="heart" />
      </span>
      <span className="today-tile-text">
        <span className="label">{t("moments.gratitude.eyebrow")}</span>
        <span className="headline">{t("today.thanks")}</span>
      </span>
      <Icon name="chevron" size="sm" />
    </button>
  );
}

const usTabs = ["moments", "questions"] as const;

export function Us() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const { household, me } = useHome();
  const [sheet, setSheet] = useState<Moment | "new" | null>(null);
  const [generation, setGeneration] = useState(0);
  const tab = usTabs.find((k) => k === params.get("show")) ?? "moments";
  const add = (
    <button type="button" className="screen-action screen-add" aria-label={t("moments.add")} onClick={() => setSheet("new")}>
      <Icon name="plus" />
    </button>
  );
  return (
    <Screen
      title={t("tabs.us")}
      action={
        <>
          {tab === "moments" && add}
          <CallButton />
        </>
      }
    >
      {household.members.length < 2 && (
        <Card
          tone="sunken"
          title={t("us.alone.title")}
          footer={
            <Button size="sm" onClick={() => navigate("/settings")}>
              {t("invite.create")}
            </Button>
          }
        >
          {t("us.alone.body")}
        </Card>
      )}
      <div className="segmented" role="tablist" aria-label={t("tabs.us")}>
        {usTabs.map((k) => (
          <button key={k} type="button" role="tab" aria-selected={tab === k} onClick={() => setParams({ show: k }, { replace: true })}>
            {t(`us.tabs.${k}`)}
          </button>
        ))}
      </div>
      <ListRow leading={<Icon name="sparkle" />} title={t("watch.title")} subtitle={t("watch.teaser")} onClick={() => navigate("/watch")} />
      {tab === "moments" ? (
        <Timeline key={generation} household={household} me={me} month={params.get("month") ?? undefined} onOpen={setSheet} />
      ) : (
        <Questions household={household} me={me} />
      )}
      {sheet && (
        <MomentSheet
          household={household}
          me={me}
          moment={sheet === "new" ? undefined : sheet}
          onClose={() => setSheet(null)}
          onSaved={() => {
            setSheet(null);
            setGeneration((g) => g + 1);
          }}
        />
      )}
    </Screen>
  );
}

function Questions({ household, me }: { household: HouseholdDetail; me: User }) {
  const { t, i18n } = useTranslation();
  const { questions, failed, reload, replace } = useQuestions(household.id);
  const day = new Intl.DateTimeFormat(i18n.language, { day: "numeric", month: "long" });
  const asked = (q: Question) => day.format(new Date(q.createdAt));
  return (
    <>
      <AskForm householdId={household.id} onAsked={reload} />
      {failed && (
        <p className="callout error" role="alert">
          {t("questions.failed")}
        </p>
      )}
      {questions?.length === 0 && (
        <Empty title={t("us.title")}>{t("us.body")}</Empty>
      )}
      {questions?.map((q) => (
        <QuestionCard key={q.id} question={q} household={household} me={me} eyebrow={asked(q)} onChange={replace} />
      ))}
    </>
  );
}
