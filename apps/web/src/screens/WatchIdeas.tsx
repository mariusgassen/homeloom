import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type WatchIdea, type WatchTitle } from "../api/client";
import { Button } from "../ui/Button";
import { Poster, TitleSheet } from "./Watch";

// Ideas for tonight from the assistant: mostly from your libraries, a few from outside them.
// Works without a library too; then every idea is from outside.
export function WatchIdeas({ householdId }: { householdId: string }) {
  const { t } = useTranslation();
  const [mood, setMood] = useState("");
  const [ideas, setIdeas] = useState<WatchIdea[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState<WatchTitle | null>(null);

  async function ask(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const res = await api
      .POST("/v1/households/{householdId}/watch/suggestions", { params: { path: { householdId } }, body: { mood: mood.trim() } })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) setIdeas(res.data.ideas);
    else setError(res?.error?.message ?? t("watch.failed"));
  }

  return (
    <section className="group watch-ideas">
      <form className="hl-card" onSubmit={ask}>
        <label className="field">
          <span className="label">{t("watch.ideas.title")}</span>
          <input className="input" value={mood} onChange={(e) => setMood(e.target.value)} placeholder={t("watch.ideas.placeholder")} maxLength={200} enterKeyHint="go" />
        </label>
        <div className="hl-card-footer">
          <Button type="submit" size="sm" icon="sparkle" disabled={busy}>
            {t(busy ? "watch.ideas.thinking" : ideas ? "watch.ideas.more" : "watch.ideas.ask")}
          </Button>
        </div>
        {error && (
          <p className="callout error" role="alert">
            {error}
          </p>
        )}
      </form>
      {ideas?.map((x) =>
        x.title ? (
          <button key={x.title.id} type="button" className="watch-idea" onClick={() => setOpen(x.title ?? null)}>
            <Poster householdId={householdId} title={x.title} />
            <IdeaText idea={x} />
          </button>
        ) : (
          <div key={x.name} className="watch-idea">
            <span className="watch-poster watch-poster-none" aria-hidden="true">
              {x.name.slice(0, 1)}
            </span>
            <IdeaText idea={x} />
          </div>
        ),
      )}
      {open && <TitleSheet householdId={householdId} title={open} onClose={() => setOpen(null)} />}
    </section>
  );
}

function IdeaText({ idea }: { idea: WatchIdea }) {
  const { t } = useTranslation();
  const facts = [idea.year, t(`watch.ideas.kind.${idea.kind}`), !idea.title && t("watch.ideas.elsewhere")].filter(Boolean);
  return (
    <span className="watch-idea-text">
      <span className="headline">{idea.name}</span>
      <span className="caption muted">{facts.join(" · ")}</span>
      <span className="body">{idea.reason}</span>
    </span>
  );
}
