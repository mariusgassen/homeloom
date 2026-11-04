import { useCallback, useEffect, useState } from "react";
import { api, type Gifts } from "./api/client";
import { local } from "./outbox";
import { useRefresh } from "./refresh";

// How many days ahead a birthday is shown on Today, with the number of ideas for it.
export const giftHorizonDays = 21;

// The caller's own gift ideas and the birthdays they are for. The API only ever returns the
// caller's own ideas, so nothing here needs to hide anything.
export function useGifts(householdId: string) {
  const [gifts, setGifts] = useState<Gifts | null>(() => local<Gifts>(`/v1/households/${householdId}/gifts`) ?? null);
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/gifts", { params: { path: { householdId } } })
      .then((res) => {
        if (!current) return;
        if (res.data) setGifts(res.data);
        setFailed(!res.data);
      })
      .catch(() => current && setFailed(true));
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, generation, refreshed, track]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  return { gifts, failed, reload };
}

// A birthday as a date input knows it: month and day in a leap year, so 29 February fits.
export const birthdayValue = (b: { month: number; day: number }) => `2000-${String(b.month).padStart(2, "0")}-${String(b.day).padStart(2, "0")}`;

// The month and day of a date input's value, or null while it isn't a full date.
export function parseBirthday(value: string): { month: number; day: number } | null {
  const m = /^\d{4}-(\d{2})-(\d{2})$/.exec(value);
  return m ? { month: Number(m[1]), day: Number(m[2]) } : null;
}
