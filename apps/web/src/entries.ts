import { useCallback, useEffect, useState } from "react";
import { api, type Entry, type EntryKind } from "./api/client";

// The household's entries of one kind, or all of them, with helpers to keep the list current.
export function useEntries(householdId: string, kind?: EntryKind) {
  const [entries, setEntries] = useState<Entry[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);

  useEffect(() => {
    let current = true;
    api
      .GET("/v1/households/{householdId}/entries", { params: { path: { householdId }, query: kind ? { kind } : {} } })
      .then((res) => {
        if (!current) return;
        if (res.data) setEntries(res.data);
        else setFailed(true);
      })
      .catch(() => current && setFailed(true));
    return () => {
      current = false;
    };
  }, [householdId, kind, generation]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  return { entries, failed, reload };
}

export async function setDone(entry: Entry, done: boolean) {
  const params = { params: { path: { entryId: entry.id } } };
  const res = await (done ? api.PUT("/v1/entries/{entryId}/done", params) : api.DELETE("/v1/entries/{entryId}/done", params)).catch(() => undefined);
  return res?.data;
}

// <input type="datetime-local"> and <input type="date"> speak local wall-clock time without a zone.
const pad = (n: number) => String(n).padStart(2, "0");

export function toLocalDate(iso: string) {
  const d = new Date(iso);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function toLocalDateTime(iso: string) {
  const d = new Date(iso);
  return `${toLocalDate(iso)}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fromLocal(value: string) {
  // A bare date means that day; noon keeps it on the same day in any nearby time zone.
  return new Date(value.length === 10 ? `${value}T12:00` : value).toISOString();
}

export function startOfDay(d: Date) {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

export function sameDay(a: Date, b: Date) {
  return startOfDay(a).getTime() === startOfDay(b).getTime();
}

// An entry is due today when it is an open reminder due before tomorrow, or an event on today.
export function dueToday(e: Entry, now: Date) {
  const tomorrow = new Date(startOfDay(now).getTime());
  tomorrow.setDate(tomorrow.getDate() + 1);
  if (e.kind === "reminder") return !e.doneAt && !!e.dueAt && new Date(e.dueAt) < tomorrow;
  if (e.kind !== "event" || !e.startsAt) return false;
  const start = new Date(e.startsAt);
  const end = new Date(e.endsAt ?? e.startsAt);
  return start < tomorrow && (e.allDay ? end >= startOfDay(now) : end >= now);
}
