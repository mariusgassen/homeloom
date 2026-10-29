import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type Entry, type EntryFields, type EntryKind, type HouseholdDetail, type Repeat, type User, type Visibility } from "../api/client";
import { fromLocal, matching, toLocalDate, toLocalDateTime, useSuggestions } from "../entries";
import { Button } from "../ui/Button";
import { Icon } from "../ui/Icon";
import { Suggestions } from "./Suggestions";

const repeats: Repeat[] = ["never", "daily", "weekly", "monthly", "yearly"];
const alerts = [0, 10, 30, 60, 24 * 60];
const maxText: Record<EntryKind, number> = { note: 5000, reminder: 200, event: 200 };

// The quick-add sheet from the design: what it is, the text, its time, and who can see it.
// Given a kind, as from that list's add, it makes just that; without one it offers the choice.
// With an entry it edits that entry instead, and the kind is fixed.
export function EntrySheet({ household, me, entry, kind: fixedKind, onClose, onSaved }: {
  household: HouseholdDetail;
  me: User;
  entry?: Entry;
  kind?: EntryKind;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<EntryKind>(entry?.kind ?? fixedKind ?? "reminder");
  const [text, setText] = useState(entry?.text ?? "");
  const [visibility, setVisibility] = useState<Visibility>(entry?.visibility ?? "shared");
  const [revealAt, setRevealAt] = useState(entry?.revealAt ? toLocalDate(entry.revealAt) : "");
  const [dueAt, setDueAt] = useState(entry?.kind === "reminder" && entry.dueAt ? toLocalDateTime(entry.dueAt) : "");
  const [repeat, setRepeat] = useState<Repeat>(entry?.repeat ?? "never");
  const [assignee, setAssignee] = useState(entry?.assigneeId ?? "");
  const [allDay, setAllDay] = useState(entry?.allDay ?? false);
  const [start, setStart] = useState(entry?.startsAt ? (entry.allDay ? toLocalDate(entry.startsAt) : toLocalDateTime(entry.startsAt)) : "");
  const [end, setEnd] = useState(entry?.endsAt ? (entry.allDay ? toLocalDate(entry.endsAt) : toLocalDateTime(entry.endsAt)) : "");
  const [alert, setAlert] = useState(entry?.alertMinutes !== undefined ? String(entry.alertMinutes) : "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const textRef = useRef<HTMLTextAreaElement & HTMLInputElement>(null);
  const author = entry ? entry.authorId === me.id : true;
  const partner = household.members.find((m) => m.user.id !== me.id)?.user;
  const suggestions = useSuggestions(household.id, "reminders");

  useEffect(() => textRef.current?.focus(), [kind]);

  // Switching all-day keeps the chosen days.
  function toggleAllDay() {
    setStart((s) => (allDay ? (s ? `${s}T09:00` : "") : s.slice(0, 10)));
    setEnd((s) => (allDay ? (s ? `${s}T10:00` : "") : s.slice(0, 10)));
    setAllDay(!allDay);
  }

  function fields(): EntryFields {
    const f: EntryFields = { text: text.trim(), visibility, repeat: "never" };
    if (visibility === "reveal" && revealAt) f.revealAt = fromLocal(`${revealAt}T00:00`);
    if (kind === "reminder") {
      if (dueAt) f.dueAt = fromLocal(dueAt);
      f.repeat = dueAt ? repeat : "never";
      if (assignee && visibility === "shared") f.assigneeId = assignee;
    }
    if (kind === "event") {
      f.allDay = allDay;
      if (start) f.startsAt = fromLocal(start);
      if (end) f.endsAt = fromLocal(end);
      if (alert) f.alertMinutes = Number(alert);
      f.repeat = repeat;
    }
    return f;
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const body = fields();
    const res = await (entry
      ? api.PUT("/v1/entries/{entryId}", { params: { path: { entryId: entry.id } }, body })
      : api.POST("/v1/households/{householdId}/entries", { params: { path: { householdId: household.id } }, body: { ...body, kind } })
    ).catch(() => undefined);
    setBusy(false);
    if (res?.data) onSaved();
    else setError(res?.error?.message ?? t("entries.failed"));
  }

  async function remove() {
    if (!entry || !window.confirm(t("entries.deleteConfirm"))) return;
    setBusy(true);
    const res = await api.DELETE("/v1/entries/{entryId}", { params: { path: { entryId: entry.id } } }).catch(() => undefined);
    setBusy(false);
    if (res?.response.ok) onSaved();
    else setError(res?.error?.message ?? t("entries.failed"));
  }

  const visibilities: Visibility[] = kind === "note" ? ["private", "shared", "reveal"] : ["private", "shared"];
  return (
    <div className="sheet-backdrop" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <form className="sheet" role="dialog" aria-modal="true" aria-label={t(`entries.title.${kind}`)} onSubmit={submit}>
        <div className="sheet-grabber" aria-hidden="true" />
        <div className="sheet-bar">
          <Button variant="plain" onClick={onClose}>
            {t("entries.cancel")}
          </Button>
          <h2 className="headline sheet-title">{t(entry ? `entries.edit.${kind}` : `entries.title.${kind}`)}</h2>
          <Button type="submit" size="sm" disabled={busy || !text.trim() || (kind === "event" && !start) || (visibility === "reveal" && !revealAt)}>
            {t("entries.save")}
          </Button>
        </div>

        {!entry && !fixedKind && (
          <div className="cook-chips" role="radiogroup" aria-label={t("entries.kind")}>
            {(["reminder", "event", "note"] as const).map((k) => (
              <button key={k} type="button" role="radio" aria-checked={kind === k} className={kind === k ? "hl-chip hl-chip-shared" : "hl-chip"} onClick={() => {
                setKind(k);
                if (k !== "note" && visibility === "reveal") setVisibility("shared");
              }}>
                {t(`entries.kinds.${k}`)}
              </button>
            ))}
          </div>
        )}

        <label className="field">
          <span className="label">{t(`entries.text.${kind}`)}</span>
          {kind === "note" ? (
            <textarea ref={textRef} className="input textarea" rows={5} value={text} maxLength={maxText.note} onChange={(e) => setText(e.target.value)} required />
          ) : (
            <input ref={textRef} className="input" value={text} maxLength={maxText[kind]} onChange={(e) => setText(e.target.value)} placeholder={t(`entries.placeholder.${kind}`)} required />
          )}
        </label>
        {!entry && kind === "reminder" && (
          <Suggestions
            items={matching(suggestions, text)}
            onPick={(s) => {
              setText(s);
              textRef.current?.focus();
            }}
          />
        )}

        {kind === "reminder" && (
          <div className="sheet-row">
            <label className="field">
              <span className="label">{t("entries.due")}</span>
              <input className="input" type="datetime-local" value={dueAt} onChange={(e) => setDueAt(e.target.value)} />
            </label>
            <label className="field">
              <span className="label">{t("entries.repeat")}</span>
              <select className="input" value={repeat} disabled={!dueAt} onChange={(e) => setRepeat(e.target.value as Repeat)}>
                {repeats.map((r) => (
                  <option key={r} value={r}>
                    {t(`entries.repeats.${r}`)}
                  </option>
                ))}
              </select>
            </label>
          </div>
        )}

        {kind === "event" && (
          <>
            <label className="check-row sheet-toggle">
              <input type="checkbox" checked={allDay} onChange={toggleAllDay} />
              <span>{t("entries.allDay")}</span>
            </label>
            <div className="sheet-row">
              <label className="field">
                <span className="label">{t("entries.starts")}</span>
                <input className="input" type={allDay ? "date" : "datetime-local"} value={start} onChange={(e) => setStart(e.target.value)} required />
              </label>
              <label className="field">
                <span className="label">{t("entries.ends")}</span>
                <input className="input" type={allDay ? "date" : "datetime-local"} value={end} min={start} onChange={(e) => setEnd(e.target.value)} />
              </label>
            </div>
            <div className="sheet-row">
              <label className="field">
                <span className="label">{t("entries.alert")}</span>
                <select className="input" value={alert} onChange={(e) => setAlert(e.target.value)}>
                  <option value="">{t("entries.alerts.none")}</option>
                  {alerts.map((m) => (
                    <option key={m} value={m}>
                      {t(`entries.alerts.${m}`)}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span className="label">{t("entries.repeat")}</span>
                <select className="input" value={repeat} onChange={(e) => setRepeat(e.target.value as Repeat)}>
                  {repeats.map((r) => (
                    <option key={r} value={r}>
                      {t(`entries.repeats.${r}`)}
                    </option>
                  ))}
                </select>
              </label>
            </div>
          </>
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

        {kind === "reminder" && visibility === "shared" && household.members.length > 1 && (
          <label className="field">
            <span className="label">{t("entries.for")}</span>
            <select className="input" value={assignee} onChange={(e) => setAssignee(e.target.value)}>
              <option value="">{t("entries.forBoth")}</option>
              {household.members.map((m) => (
                <option key={m.user.id} value={m.user.id}>
                  {m.user.id === me.id ? t("entries.forMe") : m.user.name}
                </option>
              ))}
            </select>
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
