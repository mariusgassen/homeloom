import { useCallback, useEffect, useState } from "react";
import { api, type Chore, type ChoreCompletion, type ChoreFairness } from "./api/client";
import { toLocalDate } from "./entries";
import { local } from "./outbox";
import { useRefresh } from "./refresh";

// The household's chores still to do, what was done lately and who did how much. Each part is
// loaded only when asked for, so Today reads just the chores.
export function useChores(householdId: string, { history = false } = {}) {
  const [chores, setChores] = useState<Chore[] | null>(() => local<Chore[]>(`/v1/households/${householdId}/chores`) ?? null);
  const [done, setDone] = useState<ChoreCompletion[] | null>(() => (history ? (local<ChoreCompletion[]>(`/v1/households/${householdId}/chore-completions`) ?? null) : null));
  const [fairness, setFairness] = useState<ChoreFairness | null>(() => (history ? (local<ChoreFairness>(`/v1/households/${householdId}/chore-fairness`) ?? null) : null));
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const path = { params: { path: { householdId } } };
    const request = Promise.all([
      api.GET("/v1/households/{householdId}/chores", path),
      history ? api.GET("/v1/households/{householdId}/chore-completions", path) : undefined,
      history ? api.GET("/v1/households/{householdId}/chore-fairness", path) : undefined,
    ])
      .then(([cs, ds, f]) => {
        if (!current) return;
        if (cs.data) setChores(cs.data);
        if (ds?.data) setDone(ds.data);
        if (f?.data) setFairness(f.data);
        setFailed(!cs.data || (history && (!ds?.data || !f?.data)));
      })
      .catch(() => current && setFailed(true));
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, history, generation, refreshed, track]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  return { chores, done, fairness, failed, reload };
}

// Today's date as the API writes chore days, in the phone's time zone.
export const today = () => toLocalDate(new Date().toISOString());

// A chore is on for today once its day has come; one from an earlier day is overdue.
export const dueToday = (c: Chore) => c.dueOn <= today();

// Ticks a chore off as done by you; the error's message when it can't be.
export async function completeChore(householdId: string, chore: Chore) {
  const res = await api.POST("/v1/households/{householdId}/chores/{choreId}/done", { params: { path: { householdId, choreId: chore.id } } }).catch(() => undefined);
  return res?.data ? null : (res?.error?.message ?? "");
}
