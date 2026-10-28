import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type ChatMessage, type Conversation } from "../api/client";
import { chat, toolStep } from "../assistant";
import { useCall } from "../call";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";

type Bubble = Pick<ChatMessage, "role" | "text"> & { id: string };

// Ask Homeloom: a full-screen chat, opened from the sparkle in every header. onClose says whether
// the assistant used a tool, so the screen behind it can load what may have changed.
export function Assistant({ householdId, onClose }: { householdId: string; onClose: (changed: boolean) => void }) {
  const { t, i18n } = useTranslation();
  const { talkToHomeloom } = useCall();
  const [recent, setRecent] = useState<Conversation[] | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [conversation, setConversation] = useState<string | null>(null);
  const [bubbles, setBubbles] = useState<Bubble[]>([]);
  const [step, setStep] = useState<string | null>(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const changed = useRef(false);
  const end = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    let live = true;
    api.GET("/v1/households/{householdId}/conversations", { params: { path: { householdId } } }).then(
      (res) => {
        if (!live) return;
        if (res.data) setRecent(res.data);
        else setUnavailable(true);
      },
      () => live && setUnavailable(true),
    );
    return () => {
      live = false;
    };
  }, [householdId]);

  useEffect(() => end.current?.scrollIntoView?.({ block: "end" }), [bubbles, step]);

  async function open(c: Conversation) {
    const res = await api.GET("/v1/conversations/{conversationId}", { params: { path: { conversationId: c.id } } }).catch(() => undefined);
    if (!res?.data) return setError(t("assistant.failed"));
    setConversation(c.id);
    setBubbles(res.data.messages);
    setError(null);
  }

  function restart() {
    setConversation(null);
    setBubbles([]);
    setError(null);
    input.current?.focus();
  }

  // Homeloom may act during the call, so the screen behind loads again afterwards.
  function call() {
    changed.current = true;
    talkToHomeloom(householdId);
  }

  async function send(e: FormEvent) {
    e.preventDefault();
    const message = text.trim();
    if (!message || busy) return;
    setText("");
    setBusy(true);
    setError(null);
    const reply = `reply-${Date.now()}`;
    setBubbles((bs) => [...bs, { id: `ask-${Date.now()}`, role: "user", text: message }, { id: reply, role: "assistant", text: "" }]);
    const write = (f: (b: Bubble) => Bubble) => setBubbles((bs) => bs.map((b) => (b.id === reply ? f(b) : b)));
    // Text after a tool is a new round of the answer: a new paragraph, as the stored answer has it.
    let afterTool = false;
    await chat(householdId, { text: message, ...(conversation ? { conversationId: conversation } : {}) }, {
      conversation: (c) => setConversation(c.id),
      tool: (name) => {
        changed.current = true;
        afterTool = true;
        setStep(t(toolStep(name)));
      },
      text: (piece) => {
        setStep(null);
        const gap = afterTool;
        afterTool = false;
        write((b) => ({ ...b, text: b.text && gap ? `${b.text}\n\n${piece}` : b.text + piece }));
      },
      done: (m) => write(() => m),
      error: (err) => setError(err.message || t("assistant.failed")),
    });
    setBubbles((bs) => bs.filter((b) => b.text !== ""));
    setStep(null);
    setBusy(false);
  }

  return (
    <div className="assistant" role="dialog" aria-modal="true" aria-label={t("assistant.title")} lang={i18n.language}>
      <header className="assistant-bar">
        <Button variant="plain" onClick={() => onClose(changed.current)}>
          {t("assistant.done")}
        </Button>
        <h2 className="headline sheet-title">{t("assistant.title")}</h2>
        {!unavailable && (
          <button type="button" className="screen-action" aria-label={t("assistant.call")} onClick={call}>
            <Icon name="phone" />
          </button>
        )}
        {bubbles.length > 0 ? (
          <button type="button" className="screen-action" aria-label={t("assistant.new")} onClick={restart}>
            <Icon name="plus" />
          </button>
        ) : (
          <span className="assistant-spacer" />
        )}
      </header>

      <main className="assistant-main" aria-live="polite">
        {unavailable && (
          <Card tone="sunken" title={t("assistant.unavailable.title")}>
            {t("assistant.unavailable.body")}
          </Card>
        )}
        {!unavailable && bubbles.length === 0 && (
          <>
            <p className="caption assistant-intro">{t("assistant.intro")}</p>
            {recent && recent.length > 0 && (
              <section className="group">
                <h3 className="label group-title">{t("assistant.recent")}</h3>
                <div className="hl-list">
                  {recent.slice(0, 5).map((c) => (
                    <ListRow key={c.id} title={c.title} onClick={() => open(c)} />
                  ))}
                </div>
              </section>
            )}
          </>
        )}
        {bubbles.filter((b) => b.text).map((b) => (
          <p key={b.id} className={b.role === "user" ? "bubble bubble-me" : "bubble"}>
            {b.text}
          </p>
        ))}
        {step && <p className="caption assistant-step">{step}</p>}
        {error && (
          <p className="callout error" role="alert">
            {error}
          </p>
        )}
        <div ref={end} />
      </main>

      <form className="assistant-input" onSubmit={send}>
        <input
          ref={input}
          className="input"
          value={text}
          maxLength={2000}
          disabled={unavailable}
          onChange={(e) => setText(e.target.value)}
          placeholder={t("assistant.placeholder")}
          aria-label={t("assistant.message")}
        />
        <button type="submit" className="assistant-send" aria-label={t("assistant.send")} disabled={busy || unavailable || !text.trim()}>
          <Icon name="send" />
        </button>
      </form>
    </div>
  );
}
