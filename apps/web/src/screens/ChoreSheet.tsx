import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type Chore, type ChoreFields, type HouseholdDetail, type Repeat, type User } from "../api/client";
import { today } from "../chores";
import { Button } from "../ui/Button";
import { DateRow, FormBlock, FormGroup, Pills, Segmented, SwitchRow } from "../ui/Form";
import { Sheet } from "../ui/Sheet";

const repeats: Repeat[] = ["daily", "weekly", "monthly", "yearly", "never"];
const sizes = ["1", "2", "3"] as const;

// Setting up or changing a chore: what it is, how big, how often, from when, and whose it is:
// anyone's, one person's, or yours in turns.
export function ChoreSheet({ household, me, chore, onClose, onSaved }: {
  household: HouseholdDetail;
  me: User;
  chore?: Chore;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const [title, setTitle] = useState(chore?.title ?? "");
  const [points, setPoints] = useState(String(chore?.points ?? 1) as (typeof sizes)[number]);
  const [repeat, setRepeat] = useState<Repeat>(chore?.repeat ?? "weekly");
  const [rotate, setRotate] = useState(chore?.rotate ?? household.members.length > 1);
  const [assignee, setAssignee] = useState(chore?.assigneeId ?? (household.members.length > 1 ? "" : me.id));
  const [dueOn, setDueOn] = useState(chore?.dueOn ?? today());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const titleRef = useRef<HTMLInputElement>(null);
  const together = household.members.length > 1;

  useEffect(() => {
    if (!chore) titleRef.current?.focus();
  }, [chore]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const body: ChoreFields = { title: title.trim(), points: Number(points), repeat, rotate: together && rotate, dueOn };
    // Taking turns starts with whoever is picked, you unless you pick someone else.
    if (body.rotate) body.assigneeId = assignee || me.id;
    else if (assignee) body.assigneeId = assignee;
    const path = { householdId: household.id };
    const res = await (chore
      ? api.PUT("/v1/households/{householdId}/chores/{choreId}", { params: { path: { ...path, choreId: chore.id } }, body })
      : api.POST("/v1/households/{householdId}/chores", { params: { path }, body })
    ).catch(() => undefined);
    setBusy(false);
    if (res?.data) onSaved();
    else setError(res?.error?.message ?? t("chores.failed"));
  }

  async function remove() {
    if (!chore || !window.confirm(t("chores.deleteConfirm"))) return;
    setBusy(true);
    const res = await api.DELETE("/v1/households/{householdId}/chores/{choreId}", { params: { path: { householdId: household.id, choreId: chore.id } } }).catch(() => undefined);
    setBusy(false);
    if (res?.response.ok) onSaved();
    else setError(res?.error?.message ?? t("chores.failed"));
  }

  const members = household.members.map((m) => ({ value: m.user.id, label: m.user.id === me.id ? t("chores.me") : m.user.name }));
  return (
    <Sheet
      title={t(chore ? "chores.edit" : "chores.new")}
      onClose={onClose}
      onSubmit={submit}
      leading={
        <Button variant="plain" onClick={onClose}>
          {t("entries.cancel")}
        </Button>
      }
      trailing={
        <Button type="submit" size="sm" disabled={busy || !title.trim()}>
          {t("entries.save")}
        </Button>
      }
    >
      <label className="field">
        <span className="label">{t("chores.what")}</span>
        <input ref={titleRef} className="input" value={title} maxLength={100} onChange={(e) => setTitle(e.target.value)} placeholder={t("chores.placeholder")} enterKeyHint="done" required />
      </label>

      <FormGroup>
        <FormBlock label={t("chores.size")}>
          <Segmented label={t("chores.size")} value={points} options={sizes.map((p) => ({ value: p, label: t(`chores.sizes.${p}`) }))} onChange={setPoints} />
        </FormBlock>
        <FormBlock label={t("entries.repeat")}>
          <Pills label={t("entries.repeat")} value={repeat} options={repeats.map((r) => ({ value: r, label: t(r === "never" ? "chores.once" : `entries.repeats.${r}`) }))} onChange={setRepeat} />
        </FormBlock>
        <DateRow label={t(chore ? "chores.next" : "chores.from")} type="date" value={dueOn} onChange={setDueOn} placeholder={t("entries.pick")} required />
      </FormGroup>

      {together && (
        <FormGroup>
          <SwitchRow label={t("chores.rotate")} checked={rotate} onChange={setRotate} />
          <FormBlock label={t(rotate ? "chores.turn" : "chores.for")}>
            <Segmented
              label={t(rotate ? "chores.turn" : "chores.for")}
              value={rotate ? assignee || me.id : assignee}
              options={rotate ? members : [{ value: "", label: t("chores.anyone") }, ...members]}
              onChange={setAssignee}
            />
          </FormBlock>
        </FormGroup>
      )}

      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {chore && (
        <Button variant="danger" block disabled={busy} onClick={remove}>
          {t("chores.delete")}
        </Button>
      )}
    </Sheet>
  );
}
