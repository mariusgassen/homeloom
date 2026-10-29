import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type Entry, type EntryFields, type EntryKind, type HouseholdDetail, type Repeat, type User, type Visibility } from "../api/client";
import { fromLocal, matching, toLocalDate, toLocalDateTime, useSuggestions } from "../entries";
import { Button } from "../ui/Button";
import { DateRow, FormBlock, FormGroup, Pills, Segmented, SwitchRow } from "../ui/Form";
import { Sheet } from "../ui/Sheet";
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

  // Only a new entry opens the keyboard; opening one to read or change it shouldn't hide half the sheet.
  useEffect(() => {
    if (!entry) textRef.current?.focus();
  }, [entry, kind]);

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
  const repeatPills = (
    <FormBlock label={t("entries.repeat")}>
      <Pills label={t("entries.repeat")} value={repeat} options={repeats.map((r) => ({ value: r, label: t(`entries.repeats.${r}`) }))} onChange={setRepeat} />
    </FormBlock>
  );
  return (
    <Sheet
      title={t(entry ? `entries.edit.${kind}` : `entries.title.${kind}`)}
      onClose={onClose}
      onSubmit={submit}
      leading={
        <Button variant="plain" onClick={onClose}>
          {t("entries.cancel")}
        </Button>
      }
      trailing={
        <Button type="submit" size="sm" disabled={busy || !text.trim() || (kind === "event" && !start) || (visibility === "reveal" && !revealAt)}>
          {t("entries.save")}
        </Button>
      }
    >
      {!entry && !fixedKind && (
        <Segmented
          label={t("entries.kind")}
          value={kind}
          options={(["reminder", "event", "note"] as const).map((k) => ({ value: k, label: t(`entries.kinds.${k}`) }))}
          onChange={(k) => {
            setKind(k);
            if (k !== "note" && visibility === "reveal") setVisibility("shared");
          }}
        />
      )}

      <label className="field">
        <span className="label">{t(`entries.text.${kind}`)}</span>
        {kind === "note" ? (
          <textarea ref={textRef} className="input textarea" rows={5} value={text} maxLength={maxText.note} onChange={(e) => setText(e.target.value)} required />
        ) : (
          <input ref={textRef} className="input" value={text} maxLength={maxText[kind]} onChange={(e) => setText(e.target.value)} placeholder={t(`entries.placeholder.${kind}`)} enterKeyHint="done" required />
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
        <FormGroup>
          <DateRow label={t("entries.due")} type="datetime-local" value={dueAt} onChange={setDueAt} placeholder={t("entries.noTime")} />
          {dueAt && repeatPills}
          {visibility === "shared" && household.members.length > 1 && (
            <FormBlock label={t("entries.for")}>
              <Segmented
                label={t("entries.for")}
                value={assignee}
                options={[
                  { value: "", label: t("entries.forBoth") },
                  ...household.members.map((m) => ({ value: m.user.id, label: m.user.id === me.id ? t("entries.forMe") : m.user.name })),
                ]}
                onChange={setAssignee}
              />
            </FormBlock>
          )}
        </FormGroup>
      )}

      {kind === "event" && (
        <>
          <FormGroup>
            <SwitchRow label={t("entries.allDay")} checked={allDay} onChange={toggleAllDay} />
            <DateRow label={t("entries.starts")} type={allDay ? "date" : "datetime-local"} value={start} onChange={setStart} placeholder={t("entries.pick")} required />
            <DateRow label={t("entries.ends")} type={allDay ? "date" : "datetime-local"} value={end} min={start} onChange={setEnd} placeholder={t("entries.noTime")} />
          </FormGroup>
          <FormGroup>
            <FormBlock label={t("entries.alert")}>
              <Pills
                label={t("entries.alert")}
                value={alert}
                options={[{ value: "", label: t("entries.alerts.none") }, ...alerts.map((m) => ({ value: String(m), label: t(`entries.alerts.${m}`) }))]}
                onChange={setAlert}
              />
            </FormBlock>
            {repeatPills}
          </FormGroup>
        </>
      )}

      {author && (
        <div className="field">
          <span className="label">{t("entries.who")}</span>
          <Segmented
            label={t("entries.who")}
            value={visibility}
            options={visibilities.map((v) => ({ value: v, label: t(`entries.visibility.${v}`), icon: v === "private" ? "lock" : v }))}
            onChange={setVisibility}
          />
          <p className="caption muted sheet-hint">{t(`entries.visibilityHint.${visibility}`, { name: partner?.name ?? "" })}</p>
        </div>
      )}

      {visibility === "reveal" && (
        <FormGroup>
          <DateRow label={t("entries.revealAt")} type="date" value={revealAt} onChange={setRevealAt} placeholder={t("entries.pick")} required />
        </FormGroup>
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
    </Sheet>
  );
}
