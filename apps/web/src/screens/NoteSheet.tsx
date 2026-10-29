import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type Entry, type EntryFields, type HouseholdDetail, type User, type Visibility } from "../api/client";
import { fromLocal, toLocalDate } from "../entries";
import { shrink } from "../moments";
import { joinNote, splitNote, uploadNotePhoto } from "../notes";
import { Button } from "../ui/Button";
import { Icon } from "../ui/Icon";
import { MarkdownText } from "../ui/Markdown";

const maxText = 20000;
const visibilities: Visibility[] = ["private", "shared", "reveal"];

// A note: a title, Markdown below it with photos, and who can see it. A note with text opens to read;
// Write switches to editing. With no entry it starts a new note.
export function NoteSheet({ household, me, entry, onClose, onSaved }: {
  household: HouseholdDetail;
  me: User;
  entry?: Entry;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const initial = splitNote(entry?.text ?? "");
  const [title, setTitle] = useState(initial.title);
  const [body, setBody] = useState(initial.body);
  const [mode, setMode] = useState<"write" | "read">(entry?.text ? "read" : "write");
  const [visibility, setVisibility] = useState<Visibility>(entry?.visibility ?? "shared");
  const [revealAt, setRevealAt] = useState(entry?.revealAt ? toLocalDate(entry.revealAt) : "");
  // A new note is saved before its first photo goes up, so the photo has a note to belong to.
  const [made, setMade] = useState<Entry | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const titleRef = useRef<HTMLInputElement>(null);
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  const note = entry ?? made;
  const author = entry ? entry.authorId === me.id : true;
  const partner = household.members.find((m) => m.user.id !== me.id)?.user;
  const text = joinNote(title, body);

  useEffect(() => {
    if (!entry) titleRef.current?.focus();
  }, [entry]);

  function fields(): EntryFields {
    const f: EntryFields = { text, visibility, repeat: "never" };
    if (visibility === "reveal" && revealAt) f.revealAt = fromLocal(`${revealAt}T00:00`);
    return f;
  }

  async function save() {
    const body = fields();
    const res = await (note
      ? api.PUT("/v1/entries/{entryId}", { params: { path: { entryId: note.id } }, body })
      : api.POST("/v1/households/{householdId}/entries", { params: { path: { householdId: household.id } }, body: { ...body, kind: "note" } })
    ).catch(() => undefined);
    if (!res?.data) setError(res?.error?.message ?? t("entries.failed"));
    return res?.data;
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const saved = await save();
    setBusy(false);
    if (saved) onSaved();
  }

  async function addPhoto(file: File | undefined) {
    if (!file) return;
    setBusy(true);
    setError(null);
    const at = bodyRef.current?.selectionStart ?? body.length;
    let target = note;
    if (!target) {
      // A note needs text; one without any yet gets a plain title the author can change.
      if (!text) setTitle(t("notes.untitled"));
      const f = { ...fields(), text: text || joinNote(t("notes.untitled"), "") };
      const res = await api.POST("/v1/households/{householdId}/entries", { params: { path: { householdId: household.id } }, body: { ...f, kind: "note" } }).catch(() => undefined);
      target = res?.data;
      setMade(target);
    }
    const photo = target ? await shrink(file).then((small) => uploadNotePhoto(target, small), () => undefined) : undefined;
    setBusy(false);
    if (!photo) {
      setError(t("notes.photoFailed"));
      return;
    }
    const link = `![](${photo.url})`;
    setBody((b) => [b.slice(0, at).trimEnd(), link, b.slice(at).trimStart()].filter(Boolean).join("\n\n"));
    setMode("write");
  }

  async function close() {
    // Cancelling a new note drops the copy saved for its photos, so nothing is left behind.
    if (!entry && made) await api.DELETE("/v1/entries/{entryId}", { params: { path: { entryId: made.id } } }).catch(() => undefined);
    onClose();
  }

  async function remove() {
    if (!note || !window.confirm(t("entries.deleteConfirm"))) return;
    setBusy(true);
    const res = await api.DELETE("/v1/entries/{entryId}", { params: { path: { entryId: note.id } } }).catch(() => undefined);
    setBusy(false);
    if (res?.response.ok) onSaved();
    else setError(res?.error?.message ?? t("entries.failed"));
  }

  return (
    <div className="sheet-backdrop" onClick={(e) => e.target === e.currentTarget && void close()}>
      <form className="sheet" role="dialog" aria-modal="true" aria-label={t(entry ? "entries.edit.note" : "entries.title.note")} onSubmit={submit}>
        <div className="sheet-grabber" aria-hidden="true" />
        <div className="sheet-bar">
          <Button variant="plain" onClick={() => void close()}>
            {t("entries.cancel")}
          </Button>
          <h2 className="headline sheet-title">{t(entry ? "entries.edit.note" : "entries.title.note")}</h2>
          <Button type="submit" size="sm" disabled={busy || !text || text.length > maxText || (visibility === "reveal" && !revealAt)}>
            {t("entries.save")}
          </Button>
        </div>

        <div className="note-tools">
          <div className="segmented" role="tablist" aria-label={t("entries.text.note")}>
            {(["write", "read"] as const).map((m) => (
              <button key={m} type="button" role="tab" aria-selected={mode === m} onClick={() => setMode(m)}>
                {t(`notes.${m}`)}
              </button>
            ))}
          </div>
          <label className="hl-btn hl-btn-secondary hl-btn-sm" aria-disabled={busy}>
            <Icon name="camera" size="sm" />
            <span>{t("notes.photo")}</span>
            <input className="visually-hidden" type="file" accept="image/*" disabled={busy} onChange={(e) => void addPhoto(e.target.files?.[0])} />
          </label>
        </div>

        {mode === "write" ? (
          <>
            <input ref={titleRef} className="input note-title-input" value={title} placeholder={t("notes.title")} aria-label={t("notes.title")} onChange={(e) => setTitle(e.target.value.replace(/\n/g, " "))} />
            <textarea ref={bodyRef} className="input textarea note-body-input" rows={10} value={body} placeholder={t("notes.body")} aria-label={t("entries.text.note")} onChange={(e) => setBody(e.target.value)} />
          </>
        ) : (
          <div className="bubble-md note-md">
            <MarkdownText text={text} photos />
          </div>
        )}

        {author && (
          <fieldset className="sheet-fieldset">
            <legend className="label">{t("entries.who")}</legend>
            <div className="hl-list sheet-choices">
              {visibilities.map((v) => (
                <label key={v} className="hl-listrow sheet-choice">
                  <input type="radio" name="visibility" checked={visibility === v} onChange={() => setVisibility(v)} />
                  <span className="hl-listrow-main">
                    <span className="hl-listrow-title">{t(`entries.visibility.${v}`)}</span>
                    <span className="hl-listrow-sub">{t(`entries.visibilityHint.${v}`, { name: partner?.name ?? "" })}</span>
                  </span>
                  <Icon name={v === "private" ? "lock" : v} size="sm" />
                </label>
              ))}
            </div>
          </fieldset>
        )}

        {visibility === "reveal" && (
          <label className="field">
            <span className="label">{t("entries.revealAt")}</span>
            <input className="input" type="date" value={revealAt} onChange={(e) => setRevealAt(e.target.value)} required />
          </label>
        )}

        {error && (
          <p className="callout error" role="alert">
            {error}
          </p>
        )}
        {entry && (author || entry.visibility === "shared") && (
          <Button variant="danger" block disabled={busy} onClick={remove}>
            {t("entries.delete")}
          </Button>
        )}
      </form>
    </div>
  );
}
