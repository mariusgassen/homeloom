import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type HouseholdDetail, type Question, type User } from "../api/client";
import { useRefresh } from "../refresh";
import { Button } from "../ui/Button";
import { RevealCard, type Person } from "../ui/RevealCard";
import { threadFor } from "../ui/ThreadAvatar";

export function useQuestions(householdId: string) {
  const [questions, setQuestions] = useState<Question[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/questions", { params: { path: { householdId } } })
      .then((res) => {
        if (!current) return;
        if (res.data) setQuestions(res.data);
        setFailed(!res.data);
      })
      .catch(() => current && setFailed(true));
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, generation, refreshed, track]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  const replace = useCallback((q: Question) => setQuestions((qs) => qs?.map((x) => (x.id === q.id ? q : x)) ?? [q]), []);
  return { questions, failed, reload, replace };
}

function people(q: Question, household: HouseholdDetail, me: User): Person[] {
  return household.members.map((m, i) => {
    const a = q.answers.find((x) => x.userId === m.user.id);
    return {
      name: m.user.name,
      avatarUrl: m.user.avatarUrl,
      thread: threadFor(i),
      me: m.user.id === me.id,
      answered: !!a,
      answer: a?.text,
    };
  });
}

// A question with its answers and, until it opens, a form for your own answer.
export function QuestionCard({ question, household, me, eyebrow, onChange }: {
  question: Question;
  household: HouseholdDetail;
  me: User;
  eyebrow: string;
  onChange: (q: Question) => void;
}) {
  const { t } = useTranslation();
  const mine = question.answers.find((a) => a.userId === me.id);
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(mine?.text ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const showForm = !question.revealed && (!mine || editing);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const res = await api
      .PUT("/v1/questions/{questionId}/answer", { params: { path: { questionId: question.id } }, body: { text: text.trim() } })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) {
      setEditing(false);
      onChange(res.data);
    } else {
      setError(res?.error?.message ?? t("questions.failed"));
    }
  }

  return (
    <RevealCard question={question.text} eyebrow={eyebrow} revealed={question.revealed} people={people(question, household, me)}>
      {showForm ? (
        <form className="answer-form" onSubmit={submit}>
          <label className="field">
            <span className="label">{t("questions.yourAnswer")}</span>
            <textarea className="input textarea" value={text} onChange={(e) => setText(e.target.value)} maxLength={2000} rows={3} required />
          </label>
          <p className="caption muted">{t("questions.private")}</p>
          {error && (
            <p className="callout error" role="alert">
              {error}
            </p>
          )}
          <Button type="submit" block disabled={busy || !text.trim()}>
            {t(mine ? "questions.save" : "questions.send")}
          </Button>
        </form>
      ) : (
        !question.revealed && (
          <Button variant="secondary" size="sm" onClick={() => setEditing(true)}>
            {t("questions.edit")}
          </Button>
        )
      )}
    </RevealCard>
  );
}

export function AskForm({ householdId, onAsked }: { householdId: string; onAsked: (q: Question) => void }) {
  const { t } = useTranslation();
  const suggestions = t("questions.suggestions", { returnObjects: true }) as string[];
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function suggest() {
    const others = suggestions.filter((s) => s !== text);
    setText(others[Math.floor(Math.random() * others.length)] ?? "");
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const res = await api
      .POST("/v1/households/{householdId}/questions", { params: { path: { householdId } }, body: { text: text.trim() } })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) {
      setText("");
      onAsked(res.data);
    } else {
      setError(res?.error?.message ?? t("questions.failed"));
    }
  }

  return (
    <form className="hl-card" onSubmit={submit}>
      <label className="field">
        <span className="label">{t("questions.ask")}</span>
        <textarea
          className="input textarea"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={t("questions.askPlaceholder")}
          maxLength={280}
          rows={2}
          required
        />
      </label>
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      <div className="hl-card-footer">
        <Button type="submit" size="sm" disabled={busy || !text.trim()}>
          {t("questions.askSend")}
        </Button>
        <Button variant="secondary" size="sm" icon="sparkle" onClick={suggest}>
          {t("questions.suggest")}
        </Button>
      </div>
    </form>
  );
}
