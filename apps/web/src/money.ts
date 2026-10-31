import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type Balance, type HouseholdDetail } from "./api/client";
import { useRefresh } from "./refresh";

// The household keeps its money in euros, in cents.
const currency = "EUR";

export function useMoney() {
  const { i18n } = useTranslation();
  const format = new Intl.NumberFormat(i18n.language, { style: "currency", currency });
  return (cents: number) => format.format(cents / 100);
}

// Cents from what someone typed: "12,50", "12.5", "€ 12" all work. NaN when it isn't an amount.
export function parseCents(text: string) {
  const s = text.replace(/[€\s]/g, "").replace(",", ".");
  if (!/^\d+(\.\d{0,2})?$/.test(s)) return NaN;
  return Math.round(Number(s) * 100);
}

// An amount as it is typed in the app's language, for editing.
export function centsText(cents: number, lang: string) {
  const s = (cents / 100).toFixed(2);
  return lang.startsWith("de") ? s.replace(".", ",") : s;
}

// The household's balance, read again when the screen is pulled or reload is called. undefined
// while loading, null when it couldn't be read.
export function useBalance(householdId: string) {
  const [balance, setBalance] = useState<Balance | null>();
  const [generation, setGeneration] = useState(0);
  const { generation: refreshed } = useRefresh();
  useEffect(() => {
    let current = true;
    api
      .GET("/v1/households/{householdId}/balance", { params: { path: { householdId } } })
      .catch(() => undefined)
      .then((res) => current && setBalance(res?.data ?? null));
    return () => {
      current = false;
    };
  }, [householdId, generation, refreshed]);
  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  return { balance, reload };
}

// Where you stand, in one line: who owes whom how much, or that you're even.
export function useStanding(household: HouseholdDetail, meId: string) {
  const { t } = useTranslation();
  const money = useMoney();
  const name = (id: string) => household.members.find((m) => m.user.id === id)?.user.name ?? t("money.someone");
  return (balance: Balance) => {
    const mine = balance.transfers.filter((x) => x.from === meId || x.to === meId);
    const [first] = mine;
    if (!first) return t("money.even");
    if (mine.length > 1) {
      const net = balance.members.find((m) => m.userId === meId)?.cents ?? 0;
      return t(net < 0 ? "money.youOweTotal" : "money.owedTotal", { amount: money(Math.abs(net)) });
    }
    return first.from === meId
      ? t("money.youOwe", { name: name(first.to), amount: money(first.cents) })
      : t("money.owesYou", { name: name(first.from), amount: money(first.cents) });
  };
}
