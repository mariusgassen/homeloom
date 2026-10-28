import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import type { HouseholdDetail, Question, User } from "../api/client";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { AskForm, QuestionCard, useQuestions } from "./questions";
import { Screen } from "./Screen";

// The tabs hold empty states until their features land; each says what will live there.

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
          <Button size="sm" variant="secondary" onClick={() => navigate("/us")}>
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
  const date = new Intl.DateTimeFormat(i18n.language, { weekday: "long", day: "numeric", month: "long" }).format(new Date());
  return (
    <Screen title={t("tabs.today")} overline={date}>
      <TodayQuestion household={household} me={me} />
      {(["dinner", "due"] as const).map((k) => (
        <Card key={k} eyebrow={<span>{t(`today.${k}.eyebrow`)}</span>} title={t(`today.${k}.title`)}>
          {t(`today.${k}.body`)}
        </Card>
      ))}
    </Screen>
  );
}

function Placeholder({ tab }: { tab: "lists" | "home" }) {
  const { t } = useTranslation();
  return (
    <Screen title={t(`tabs.${tab}`)}>
      <Card tone="sunken" title={t(`${tab}.title`)}>
        {t(`${tab}.body`)}
      </Card>
    </Screen>
  );
}

export function Us() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { household, me } = useHome();
  const { questions, failed, reload, replace } = useQuestions(household.id);
  const day = new Intl.DateTimeFormat(i18n.language, { day: "numeric", month: "long" });
  const asked = (q: Question) => day.format(new Date(q.createdAt));
  return (
    <Screen title={t("tabs.us")}>
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
      <AskForm householdId={household.id} onAsked={reload} />
      {failed && (
        <p className="callout error" role="alert">
          {t("questions.failed")}
        </p>
      )}
      {questions?.length === 0 && (
        <Card tone="sunken" title={t("us.title")}>
          {t("us.body")}
        </Card>
      )}
      {questions?.map((q) => (
        <QuestionCard key={q.id} question={q} household={household} me={me} eyebrow={asked(q)} onChange={replace} />
      ))}
    </Screen>
  );
}
export const Lists = () => <Placeholder tab="lists" />;
export const Home = () => <Placeholder tab="home" />;
