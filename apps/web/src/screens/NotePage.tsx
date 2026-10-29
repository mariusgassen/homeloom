import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router";
import { api, type Entry, type EntryFields, type Visibility } from "../api/client";
import { fromLocal, toLocalDate } from "../entries";
import { shrink } from "../moments";
import { joinNote, splitNote, uploadNotePhoto } from "../notes";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Segmented } from "../ui/Form";
import { Icon } from "../ui/Icon";
import { MarkdownText } from "../ui/Markdown";

const visibilities: Visibility[] = ["private", "shared", "reveal"];
// How long typing pauses before the note saves on its own.
const saveAfter = 1200;
const back = "/lists?show=note";

// A note on its own page, like a notes app: a large title, Markdown below it with photos, saved on its
// own while writing and when leaving. /lists/notes/new starts one; it is made with its first words.
// A note with text opens to read; Write and Preview switch between the text and how it looks.
// Discard undoes everything since the note was opened, and a new note isn't kept.
export function NotePage() {
  const { id = "new" } = useParams();
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) throw new Error("tab without a household");
  const [loaded, setLoaded] = useState<Entry | null | undefined>(id === "new" ? null : undefined);

  useEffect(() => {
    if (id === "new") return;
    let live = true;
    api.GET("/v1/entries/{entryId}", { params: { path: { entryId: id } } }).then(
      (res) => live && setLoaded(res.data?.kind === "note" && !res.data.sealed ? res.data : null),
      () => live && setLoaded(null),
    );
    return () => {
      live = false;
    };
  }, [id]);

  if (loaded === undefined) return <div className="note-page" aria-busy="true" />;
  if (loaded === null && id !== "new") return <NoteMissing />;
  // A note made on this page gets its id in the URL; the editor keeps going as it is.
  return <NoteEditor key={loaded?.id ?? "new"} entry={loaded ?? undefined} householdId={session.household.id} meId={session.user.id} partner={session.household.members.find((m) => m.user.id !== session.user.id)?.user.name ?? ""} />;
}

function NoteMissing() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  return (
    <div className="note-page">
      <div className="note-bar">
        <BackButton onClick={() => navigate(back)} />
      </div>
      <p className="callout error">{t("notes.missing")}</p>
    </div>
  );
}

function BackButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <Button variant="plain" size="sm" className="back" onClick={onClick}>
      <span className="back-icon">
        <Icon name="chevron" size="sm" />
      </span>
      {t("entries.tabs.note")}
    </Button>
  );
}

function NoteEditor({ entry, householdId, meId, partner }: { entry?: Entry; householdId: string; meId: string; partner: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const initial = splitNote(entry?.text ?? "");
  const [title, setTitle] = useState(initial.title);
  const [body, setBody] = useState(initial.body);
  const [visibility, setVisibility] = useState<Visibility>(entry?.visibility ?? "shared");
  const [revealAt, setRevealAt] = useState(entry?.revealAt ? toLocalDate(entry.revealAt) : "");
  const [mode, setMode] = useState<"write" | "read">(entry?.text ? "read" : "write");
  const [who, setWho] = useState(false);
  const [status, setStatus] = useState<"saved" | "saving" | "failed">("saved");
  const [busy, setBusy] = useState(false);
  const note = useRef(entry);
  // What was last saved, so only a change saves again; saves run one after another.
  const opened = JSON.stringify(fieldsOf(joinNote(initial.title, initial.body), entry?.visibility ?? "shared", entry?.revealAt ? toLocalDate(entry.revealAt) : ""));
  const saved = useRef(opened);
  // Set once the note is being discarded or deleted, so a pending save doesn't bring it back.
  const closing = useRef(false);
  const queue = useRef(Promise.resolve(true));
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  const titleRef = useRef<HTMLTextAreaElement>(null);
  const editable = !entry || entry.authorId === meId || entry.visibility === "shared";
  const author = !entry || entry.authorId === meId;
  const text = joinNote(title, body);

  const save = useCallback(
    (fields: EntryFields) =>
      (queue.current = queue.current.then(async () => {
        const json = JSON.stringify(fields);
        // An empty note isn't made, and a "later" note waits for its date.
        if (json === saved.current || !fields.text || (fields.visibility === "reveal" && !fields.revealAt)) return true;
        setStatus("saving");
        const current = note.current;
        const res = await (current
          ? api.PUT("/v1/entries/{entryId}", { params: { path: { entryId: current.id } }, body: fields })
          : api.POST("/v1/households/{householdId}/entries", { params: { path: { householdId } }, body: { ...fields, kind: "note" } })
        ).catch(() => undefined);
        if (!res?.data) {
          setStatus("failed");
          return false;
        }
        saved.current = json;
        if (!current) {
          note.current = res.data;
          window.history.replaceState(window.history.state, "", `/lists/notes/${res.data.id}`);
        }
        setStatus("saved");
        return true;
      })),
    [householdId, setStatus],
  );

  const fields = fieldsOf(text, visibility, revealAt);

  // Save a moment after typing stops, and at once when the app goes to the background.
  const json = JSON.stringify(fields);
  useEffect(() => {
    if (!editable) return;
    const now = () => void (closing.current || save(JSON.parse(json) as EntryFields));
    const hide = () => document.visibilityState === "hidden" && now();
    const timer = window.setTimeout(now, saveAfter);
    document.addEventListener("visibilitychange", hide);
    return () => {
      window.clearTimeout(timer);
      document.removeEventListener("visibilitychange", hide);
    };
  }, [json, editable, save]);

  // The title and text grow with what is in them, so the page scrolls instead of the fields.
  useEffect(() => {
    for (const el of [titleRef.current, bodyRef.current]) {
      if (!el) continue;
      el.style.height = "auto";
      el.style.height = `${el.scrollHeight}px`;
    }
  }, [title, body, mode]);

  useEffect(() => {
    if (!entry) titleRef.current?.focus();
  }, [entry]);

  async function leave() {
    if (editable) {
      const current = note.current;
      if (current && !text) {
        // Emptying a note deletes it, as in a notes app.
        await api.DELETE("/v1/entries/{entryId}", { params: { path: { entryId: current.id } } }).catch(() => undefined);
      } else if (!(await save(fields)) && !window.confirm(t("notes.leaveUnsaved"))) {
        return;
      }
    }
    navigate(back);
  }

  async function addPhoto(file: File | undefined) {
    if (!file) return;
    setBusy(true);
    const at = bodyRef.current?.selectionStart ?? body.length;
    if (!note.current) {
      // A photo needs a note to belong to; one without words yet gets a plain title to change later.
      const named = title.trim() || t("notes.untitled");
      setTitle(named);
      await save(fieldsOf(joinNote(named, body), visibility, revealAt));
    }
    const target = note.current;
    const photo = target ? await shrink(file).then((small) => uploadNotePhoto(target, small), () => undefined) : undefined;
    setBusy(false);
    if (!photo) {
      setStatus("failed");
      return;
    }
    setBody((b) => [b.slice(0, at).trimEnd(), `![](${photo.url})`, b.slice(at).trimStart()].filter(Boolean).join("\n\n"));
    setMode("write");
  }

  async function remove(ask = true) {
    if (ask && !window.confirm(t("entries.deleteConfirm"))) return;
    closing.current = true;
    await queue.current;
    const current = note.current;
    if (current) {
      const res = await api.DELETE("/v1/entries/{entryId}", { params: { path: { entryId: current.id } } }).catch(() => undefined);
      if (!res?.response.ok) {
        closing.current = false;
        setStatus("failed");
        return;
      }
    }
    navigate(back);
  }

  // Puts the note back as it was when opened; a note made on this page is deleted instead.
  async function discard() {
    if (!window.confirm(t("notes.discardConfirm"))) return;
    if (!entry) return remove(false);
    closing.current = true;
    if (!(await save(JSON.parse(opened) as EntryFields))) {
      closing.current = false;
      return;
    }
    navigate(back);
  }

  return (
    <div className="note-page">
      <div className="note-bar">
        <BackButton onClick={() => void leave()} />
        {status !== "saved" && (
          <span className={status === "failed" ? "caption note-state note-state-failed" : "caption note-state"} role="status">
            {t(`notes.${status}`)}
          </span>
        )}
        {editable && (
          <>
            <label className="screen-action note-photo-button" aria-label={t("notes.photo")} aria-disabled={busy}>
              <Icon name="camera" />
              <input className="visually-hidden" type="file" accept="image/*" disabled={busy} onChange={(e) => void addPhoto(e.target.files?.[0])} />
            </label>
            {author && (
              <button type="button" className="screen-action" aria-label={t("entries.who")} aria-expanded={who} onClick={() => setWho(!who)}>
                <Icon name={visibility === "private" ? "lock" : visibility} />
              </button>
            )}
            <button type="button" className="screen-action" aria-label={t("entries.delete")} onClick={() => void remove()}>
              <Icon name="trash" />
            </button>
          </>
        )}
        {editable && (
          <div className="note-modes">
            <Segmented
              label={t("notes.mode")}
              value={mode}
              options={[
                { value: "write", label: t("notes.write"), icon: "edit" },
                { value: "read", label: t("notes.preview"), icon: "eye" },
              ]}
              onChange={setMode}
            />
            {json !== opened && (
              <Button variant="plain" size="sm" onClick={() => void discard()}>
                {t("notes.discard")}
              </Button>
            )}
          </div>
        )}
      </div>

      {who && author && (
        <div className="note-who">
          <fieldset className="sheet-fieldset">
            <legend className="label">{t("entries.who")}</legend>
            <div className="hl-list sheet-choices">
              {visibilities.map((v) => (
                <label key={v} className="hl-listrow sheet-choice">
                  <input type="radio" name="visibility" checked={visibility === v} onChange={() => setVisibility(v)} />
                  <span className="hl-listrow-main">
                    <span className="hl-listrow-title">{t(`entries.visibility.${v}`)}</span>
                    <span className="hl-listrow-sub">{t(`entries.visibilityHint.${v}`, { name: partner })}</span>
                  </span>
                  <Icon name={v === "private" ? "lock" : v} size="sm" />
                </label>
              ))}
            </div>
          </fieldset>
          {visibility === "reveal" && (
            <label className="field">
              <span className="label">{t("entries.revealAt")}</span>
              <input className="input" type="date" value={revealAt} onChange={(e) => setRevealAt(e.target.value)} required />
            </label>
          )}
        </div>
      )}

      {mode === "write" && editable ? (
        <>
          <textarea
            ref={titleRef}
            className="note-title-input large-title"
            rows={1}
            value={title}
            placeholder={t("notes.title")}
            aria-label={t("notes.title")}
            onChange={(e) => setTitle(e.target.value.replace(/\n/g, " "))}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                bodyRef.current?.focus();
              }
            }}
          />
          <textarea ref={bodyRef} className="note-body-input" rows={8} value={body} placeholder={t("notes.body")} aria-label={t("entries.text.note")} onChange={(e) => setBody(e.target.value)} />
        </>
      ) : (
        <article className="note-read" onDoubleClick={() => editable && setMode("write")}>
          {title && <h1 className="large-title note-read-title">{title}</h1>}
          <div className="bubble-md note-md">
            <MarkdownText text={body} photos />
          </div>
        </article>
      )}
    </div>
  );
}

function fieldsOf(text: string, visibility: Visibility, revealAt: string): EntryFields {
  const f: EntryFields = { text, visibility, repeat: "never" };
  if (visibility === "reveal" && revealAt) f.revealAt = fromLocal(`${revealAt}T00:00`);
  return f;
}
