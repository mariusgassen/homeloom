import { useCallback, useEffect, useState } from "react";
import { api, type Entry, type EntryKind } from "./api/client";
import { local } from "./outbox";
import { useRefresh } from "./refresh";

// The household's entries of one kind, or all of them, with helpers to keep the list current.
// kind null loads nothing, for a tab that shows something else.
export function useEntries(householdId: string, kind?: EntryKind | null) {
  const [entries, setEntries] = useState<Entry[] | null>(() => (kind === null ? null : (local<Entry[]>(`/v1/households/${householdId}/entries`, { kind }) ?? null)));
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    if (kind === null) return;
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/entries", { params: { path: { householdId }, query: kind ? { kind } : {} } })
      .then((res) => {
        if (!current) return;
        if (res.data) setEntries(res.data);
        setFailed(!res.data);
      })
      .catch(() => current && setFailed(true));
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, kind, generation, refreshed, track]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  return { entries, failed, reload };
}

export async function setDone(entry: Entry, done: boolean) {
  const params = { params: { path: { entryId: entry.id } } };
  const res = await (done ? api.PUT("/v1/entries/{entryId}/done", params) : api.DELETE("/v1/entries/{entryId}/done", params)).catch(() => undefined);
  return res?.data;
}

// Deletes an entry after asking; true once it is gone.
export async function removeEntry(entry: Entry, confirm: string) {
  if (!window.confirm(confirm)) return false;
  const res = await api.DELETE("/v1/entries/{entryId}", { params: { path: { entryId: entry.id } } }).catch(() => undefined);
  return !!res?.response.ok;
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

// Whether a list shows its done items, remembered per person on this device; hidden by default.
export function useShowDone(userId: string, list: "reminders" | "shopping") {
  const key = `homeloom.showDone.${userId}.${list}`;
  const [show, setShow] = useState(() => {
    try {
      return localStorage.getItem(key) === "1";
    } catch {
      return false;
    }
  });
  const toggle = useCallback(() => {
    setShow((s) => {
      try {
        if (s) localStorage.removeItem(key);
        else localStorage.setItem(key, "1");
      } catch {
        // Not remembered, but still toggled.
      }
      return !s;
    });
  }, [key]);
  return [show, toggle] as const;
}

// Titles used before, to use again: past reminders or shopping items. A change in generation
// loads them again. Empty while loading or when they can't be read.
export function useSuggestions(householdId: string, list: "reminders" | "shopping", generation = 0) {
  const [suggestions, setSuggestions] = useState<string[]>([]);
  useEffect(() => {
    let current = true;
    const params = { params: { path: { householdId } } };
    const request =
      list === "reminders" ? api.GET("/v1/households/{householdId}/entries/suggestions", params) : api.GET("/v1/households/{householdId}/shopping-list/suggestions", params);
    request.then((res) => current && setSuggestions(res.data?.map((s) => s.text) ?? []), () => undefined);
    return () => {
      current = false;
    };
  }, [householdId, list, generation]);
  return suggestions;
}

// The suggestions that fit what's typed so far, at most n; an exact match is left out.
export function matching(suggestions: string[], typed: string, n = 6) {
  const q = typed.trim().toLowerCase();
  return suggestions.filter((s) => s.toLowerCase() !== q && s.toLowerCase().includes(q)).slice(0, n);
}
