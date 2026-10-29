import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router";
import type { HouseholdDetail, Moment, Question, User } from "../api/client";
import { dueToday, useEntries } from "../entries";
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
  return (
    <Screen title={t("tabs.today")} overline={date} add={{ label: t("entries.add"), onClick: () => setAdding(true) }}>
      <TodayQuestion household={household} me={me} />
      <Card eyebrow={<span>{t("today.dinner.eyebrow")}</span>} title={t("today.dinner.title")}>
        {t("today.dinner.body")}
      </Card>
      <DueToday key={generation} household={household} />
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

// Open reminders due by tonight (overdue ones too) and today's events; each opens in Lists.
function DueToday({ household }: { household: HouseholdDetail }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { entries } = useEntries(household.id);
  const { span, moment } = useWhen();
  if (!entries) return null;
  const now = new Date();
  const due = entries.filter((e) => dueToday(e, now));
  if (due.length === 0) {
    return (
      <Card eyebrow={<span>{t("today.due.eyebrow")}</span>} title={t("today.due.title")}>
        {t("today.due.body")}
      </Card>
    );
  }
  return (
    <section className="group">
      <h2 className="label group-title">{t("today.due.eyebrow")}</h2>
      <div className="hl-list">
        {due.map((e) => (
          <ListRow
            key={e.id}
            leading={<Icon name={e.kind === "event" ? "calendar" : "bell"} />}
            title={e.text}
            subtitle={e.kind === "event" ? span(e) : e.dueAt && moment(e.dueAt)}
            onClick={() => navigate(`/lists?entry=${e.id}`)}
          />
        ))}
      </div>
    </section>
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
  return (
    <Screen title={t("tabs.us")} action={<CallButton />} add={tab === "moments" ? { label: t("moments.add"), onClick: () => setSheet("new") } : undefined}>
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
