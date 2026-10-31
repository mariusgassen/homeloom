import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type HouseholdDetail, type Question, type User } from "../api/client";
import { cached } from "../cache";
import { useRefresh } from "../refresh";
import { Button } from "../ui/Button";
import { RevealCard, type Person } from "../ui/RevealCard";
import { threadFor } from "../ui/ThreadAvatar";

export function useQuestions(householdId: string) {
  const [questions, setQuestions] = useState<Question[] | null>(() => cached<Question[]>(`/v1/households/${householdId}/questions`) ?? null);
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

// A question with its answers and, until it opens, a form for your own answer. With onRemoved,
// whoever asked can also reword it (until someone else answers) or delete it.
export function QuestionCard({ question, household, me, eyebrow, onChange, onRemoved }: {
  question: Question;
  household: HouseholdDetail;
  me: User;
  eyebrow: string;
  onChange: (q: Question) => void;
  onRemoved?: () => void;
}) {
  const { t } = useTranslation();
  const mine = question.answers.find((a) => a.userId === me.id);
  const [editing, setEditing] = useState(false);
  const [rewording, setRewording] = useState(false);
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

  if (rewording && onRemoved) {
    return (
      <RewordForm
        question={question}
        canReword={!question.answers.some((a) => a.userId !== me.id)}
        onDone={(q) => {
          setRewording(false);
          if (q) onChange(q);
        }}
        onRemoved={onRemoved}
      />
    );
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
      {onRemoved && question.askedBy === me.id && (
        <Button variant="plain" size="sm" className="question-manage" onClick={() => setRewording(true)}>
          {t("questions.manage")}
        </Button>
      )}
    </RevealCard>
  );
}

// Rewording your own question, or deleting it with its answers.
function RewordForm({ question, canReword, onDone, onRemoved }: {
  question: Question;
  canReword: boolean;
  onDone: (q?: Question) => void;
  onRemoved: () => void;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState(question.text);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const path = { params: { path: { questionId: question.id } } };

  async function save(e: FormEvent) {
    e.preventDefault();
    if (text.trim() === question.text) return onDone();
    setBusy(true);
    setError(null);
    const res = await api.PUT("/v1/questions/{questionId}", { ...path, body: { text: text.trim() } }).catch(() => undefined);
    setBusy(false);
    if (res?.data) onDone(res.data);
    else setError(res?.error?.message ?? t("questions.failed"));
  }

  async function remove() {
    if (!confirming) return setConfirming(true);
    setBusy(true);
    setError(null);
    const res = await api.DELETE("/v1/questions/{questionId}", path).catch(() => undefined);
    setBusy(false);
    if (res?.response.ok) onRemoved();
    else setError(res?.error?.message ?? t("questions.failed"));
  }

  return (
    <form className="hl-card" onSubmit={save}>
      {canReword ? (
        <label className="field">
          <span className="label">{t("questions.reword")}</span>
          <textarea className="input textarea" value={text} onChange={(e) => setText(e.target.value)} maxLength={280} rows={2} required autoFocus />
        </label>
      ) : (
        <>
          <h2 className="hl-reveal-q">{question.text}</h2>
          <p className="caption muted">{t("questions.answeredAlready")}</p>
        </>
      )}
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      <div className="hl-card-footer">
        {canReword && (
          <Button type="submit" size="sm" disabled={busy || !text.trim()}>
            {t("questions.save")}
          </Button>
        )}
        <Button variant="secondary" size="sm" disabled={busy} onClick={() => onDone()}>
          {t("questions.cancel")}
        </Button>
        <Button variant="danger" size="sm" disabled={busy} onClick={remove}>
          {t(confirming ? "questions.deleteConfirm" : "questions.delete")}
        </Button>
      </div>
    </form>
  );
}

export function AskForm({ householdId, onAsked }: { householdId: string; onAsked: (q: Question) => void }) {
  const { t } = useTranslation();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [thinking, setThinking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Fresh questions from the API, used up one per tap; a new batch when they run out.
  const [ideas, setIdeas] = useState<string[]>([]);

  async function suggest() {
    let next = ideas.filter((s) => s !== text);
    if (next.length === 0) {
      setThinking(true);
      const res = await api
        .GET("/v1/households/{householdId}/questions/suggestions", { params: { path: { householdId } } })
        .catch(() => undefined);
      setThinking(false);
      // Without the assistant, fall back to a few built-in questions.
      const fallback = (t("questions.suggestions", { returnObjects: true }) as string[]).filter((s) => s !== text);
      next = res?.data?.questions.length ? res.data.questions : [fallback[Math.floor(Math.random() * fallback.length)] ?? ""];
    }
    setText(next[0] ?? "");
    setIdeas(next.slice(1));
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
        <Button variant="secondary" size="sm" icon="sparkle" disabled={thinking} onClick={suggest}>
          {t(thinking ? "questions.thinking" : "questions.suggest")}
        </Button>
      </div>
    </form>
  );
}
