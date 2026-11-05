import { useCallback, useEffect, useState } from "react";
import { api, type Wish } from "./api/client";
import { local } from "./outbox";
import { useRefresh } from "./refresh";

// The household's wishlist, the most wanted first.
export function useWishes(householdId: string) {
  const [wishes, setWishes] = useState<Wish[] | null>(
    () => local<Wish[]>(`/v1/households/${householdId}/wishes`) ?? null,
  );
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/wishes", {
        params: { path: { householdId } },
      })
      .then((res) => {
        if (!current) return;
        if (res.data) setWishes(res.data);
        setFailed(!res.data);
      })
      .catch(() => current && setFailed(true));
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, generation, refreshed, track]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  return { wishes, failed, reload };
}

// Your hearts for a wish, 0 when you gave none.
export const heartsOf = (w: Wish, userId: string) =>
  w.votes.find((v) => v.userId === userId)?.hearts ?? 0;

// Gives a wish your hearts, or takes them back with 0; the error's message when it can't be.
export async function vote(householdId: string, wish: Wish, hearts: number) {
  const params = { params: { path: { householdId, wishId: wish.id } } };
  const res = await (
    hearts > 0
      ? api.PUT("/v1/households/{householdId}/wishes/{wishId}/vote", {
          ...params,
          body: { hearts },
        })
      : api.DELETE("/v1/households/{householdId}/wishes/{wishId}/vote", params)
  ).catch(() => undefined);
  return res?.data ? null : (res?.error?.message ?? "");
}

// Marks a wish as come true, or as a wish again.
export async function setGot(householdId: string, wish: Wish, got: boolean) {
  const params = { params: { path: { householdId, wishId: wish.id } } };
  const res = await (
    got
      ? api.PUT("/v1/households/{householdId}/wishes/{wishId}/got", params)
      : api.DELETE("/v1/households/{householdId}/wishes/{wishId}/got", params)
  ).catch(() => undefined);
  return res?.data ? null : (res?.error?.message ?? "");
}
