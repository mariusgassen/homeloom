import { useCallback, useEffect, useState } from "react";
import { api, type MealRating, type Rating } from "./api/client";
import { useRefresh } from "./refresh";

export const ratings: Rating[] = ["love", "fine", "nope"];

// What each of you said about a dish.
export function ratingsOf(all: MealRating[], slug: string, me: string): { mine?: Rating; theirs: MealRating[] } {
  const here = all.filter((r) => r.slug === slug);
  return { mine: here.find((r) => r.userId === me)?.rating, theirs: here.filter((r) => r.userId !== me) };
}

// The household's ratings; undefined while loading or when they can't be read. Rating changes the
// list at once and puts it back if the API says no. rate returns the error message, or null.
export function useTaste(householdId: string) {
  const [all, setAll] = useState<MealRating[]>();
  const { generation } = useRefresh();

  useEffect(() => {
    let current = true;
    api
      .GET("/v1/households/{householdId}/taste", { params: { path: { householdId } } })
      .catch(() => undefined)
      .then((res) => current && setAll(res?.data?.ratings));
    return () => {
      current = false;
    };
  }, [householdId, generation]);

  const rate = useCallback(
    async (slug: string, me: string, rating: Rating | null): Promise<string | null> => {
      const before = all;
      const without = (all ?? []).filter((r) => !(r.slug === slug && r.userId === me));
      if (rating === null) {
        setAll(without);
        const res = await api.DELETE("/v1/households/{householdId}/recipes/{slug}/rating", { params: { path: { householdId, slug } } }).catch(() => undefined);
        if (res?.response.ok) return null;
        setAll(before);
        return res?.error?.message ?? "";
      }
      const res = await api
        .PUT("/v1/households/{householdId}/recipes/{slug}/rating", { params: { path: { householdId, slug } }, body: { rating } })
        .catch(() => undefined);
      if (res?.data) {
        const saved = res.data;
        setAll([saved, ...without]);
        return null;
      }
      return res?.error?.message ?? "";
    },
    [householdId, all],
  );
  return { all, rate };
}
