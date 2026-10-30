import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type PlannedMeal } from "./api/client";
import { useRefresh } from "./refresh";

// Days are calendar dates as YYYY-MM-DD, in the device's time zone.
export function isoDay(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

// The next n days, today first.
export function nextDays(n: number, from = new Date()): string[] {
  return Array.from({ length: n }, (_, i) => isoDay(new Date(from.getFullYear(), from.getMonth(), from.getDate() + i)));
}

export function dateOf(day: string): Date {
  const [y = 0, m = 1, d = 1] = day.split("-").map(Number);
  return new Date(y, m - 1, d);
}

// The household's dinners for the days, by day; undefined while loading. Planning and taking off
// change the list at once, and put it back if the API says no.
export function useMealPlan(householdId: string, days: string[]) {
  const [meals, setMeals] = useState<Record<string, PlannedMeal>>();
  const { generation } = useRefresh();
  const from = days[0] ?? "";
  const count = days.length;

  useEffect(() => {
    let current = true;
    api
      .GET("/v1/households/{householdId}/meal-plan", { params: { path: { householdId }, query: { from, days: count } } })
      .catch(() => undefined)
      .then((res) => current && setMeals(Object.fromEntries((res?.data ?? []).map((m) => [m.day, m]))));
    return () => {
      current = false;
    };
  }, [householdId, from, count, generation]);

  // Returns the error message, or null when it worked.
  const plan = useCallback(
    async (day: string, recipe: string): Promise<string | null> => {
      const res = await api
        .PUT("/v1/households/{householdId}/meal-plan/{day}", { params: { path: { householdId, day } }, body: { recipe } })
        .catch(() => undefined);
      const meal = res?.data;
      if (meal) setMeals((m) => ({ ...m, [day]: meal }));
      return meal ? null : (res?.error?.message ?? "");
    },
    [householdId],
  );
  const unplan = useCallback(
    async (day: string) => {
      setMeals((m) => Object.fromEntries(Object.entries(m ?? {}).filter(([d]) => d !== day)));
      await api.DELETE("/v1/households/{householdId}/meal-plan/{day}", { params: { path: { householdId, day } } }).catch(() => undefined);
    },
    [householdId],
  );
  return { meals, plan, unplan };
}

// "Today", "Tomorrow", or the weekday and date (only the short weekday when short).
export function useDayLabel(short = false) {
  const { t, i18n } = useTranslation();
  const [today, tomorrow] = nextDays(2);
  return (day: string) => {
    if (day === today) return t("week.today");
    if (day === tomorrow) return t("week.tomorrow");
    const format: Intl.DateTimeFormatOptions = short ? { weekday: "short" } : { weekday: "long", day: "numeric", month: "short" };
    return new Intl.DateTimeFormat(i18n.language, format).format(dateOf(day));
  };
}
