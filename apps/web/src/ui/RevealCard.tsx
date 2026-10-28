import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Icon, cx } from "./Icon";
import { ThreadAvatar, type Thread } from "./ThreadAvatar";

export type Person = {
  name: string;
  avatarUrl?: string;
  thread?: Thread;
  me: boolean;
  answered: boolean;
  answer?: string; // only when the caller may read it
};

// The design system's RevealCard: the weave stays striped while answers are sealed and
// turns into two solid halves once the question opens.
export function RevealCard({ question, eyebrow, revealed, people, children }: {
  question: string;
  eyebrow: string;
  revealed: boolean;
  people: Person[];
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <section className="hl-reveal" aria-label={question}>
      <div className="hl-eyebrow">
        <span>{eyebrow}</span>
        <span className={cx("hl-chip", revealed ? "hl-chip-shared" : "hl-chip-reveal")}>
          <Icon name={revealed ? "shared" : "reveal"} size="sm" />
          <span>{t(revealed ? "questions.opened" : "questions.sealed")}</span>
        </span>
      </div>
      <h2 className="hl-reveal-q">{question}</h2>
      <div className={cx("hl-weave", revealed && "hl-weave-open")} aria-hidden="true" />
      <div className="hl-answers">
        {people.map((p) => (
          <div className="hl-answer" key={p.name + String(p.me)}>
            <ThreadAvatar name={p.name} src={p.avatarUrl} thread={p.thread} size="sm" />
            {p.answer !== undefined ? (
              <div>
                <p className="hl-answer-text">{p.answer}</p>
                <div className="hl-answer-meta">{p.me ? t("questions.you") : p.name}</div>
              </div>
            ) : (
              <div className="hl-sealed">
                <Icon name={p.answered ? "lock" : "reveal"} size="sm" />
                {p.answered ? t("questions.theyAnswered", { name: p.name }) : t("questions.waiting", { name: p.name })}
              </div>
            )}
          </div>
        ))}
      </div>
      {children}
    </section>
  );
}
