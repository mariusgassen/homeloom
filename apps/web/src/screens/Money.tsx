import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { api, type Expense, type ExpenseCategory, type ExpenseMonth, type HouseholdDetail, type Transfer, type User } from "../api/client";
import { categories, centsText, parseCents, useBalance, useMoney, useStanding } from "../money";
import { shrink, today } from "../moments";
import { useRefresh } from "../refresh";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { Empty } from "../ui/Empty";
import { DateRow, FormBlock, FormGroup, Pills, Segmented } from "../ui/Form";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { Sheet } from "../ui/Sheet";
import { Swipe } from "../ui/Swipe";
import { ThreadAvatar, threadFor } from "../ui/ThreadAvatar";
import { Screen } from "./Screen";

// The months shown so far, newest first; loadEarlier adds the one before the last.
function useExpenses(householdId: string) {
  const [months, setMonths] = useState<ExpenseMonth[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);
  const { generation: refreshed, track } = useRefresh();
  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/expenses", { params: { path: { householdId } } })
      .catch(() => undefined)
      .then((res) => {
        if (!current) return;
        setFailed(!res?.data);
        if (res?.data) setMonths([res.data]);
      });
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, generation, refreshed, track]);
  const earlier = months?.at(-1)?.earlier;
  const loadEarlier = async () => {
    if (!earlier) return;
    const res = await api.GET("/v1/households/{householdId}/expenses", { params: { path: { householdId }, query: { month: earlier } } }).catch(() => undefined);
    if (res?.data) setMonths((ms) => [...(ms ?? []), res.data]);
    else setFailed(true);
  };
  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  return { months, failed, earlier, loadEarlier, reload };
}

// The household's shared money: where you stand and settling up on top, then what was spent, month
// by month. Tapping an expense changes it; swiping left deletes it.
export function Money() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) throw new Error("money without a household");
  const { user: me, household } = session;
  const { balance, reload: reloadBalance } = useBalance(household.id);
  const { months, failed, earlier, loadEarlier, reload: reloadExpenses } = useExpenses(household.id);
  const [sheet, setSheet] = useState<Expense | "new" | null>(null);
  const [settling, setSettling] = useState<Transfer | null>(null);
  const standing = useStanding(household, me.id);
  const money = useMoney();
  const monthName = new Intl.DateTimeFormat(i18n.language, { month: "long", year: "numeric" });
  const name = (id: string) => household.members.find((m) => m.user.id === id)?.user.name ?? t("money.someone");
  const reload = () => {
    reloadBalance();
    reloadExpenses();
  };
  // Your own payment first: that's the one you'd settle.
  const transfer = balance?.transfers.find((x) => x.from === me.id) ?? balance?.transfers.find((x) => x.to === me.id) ?? balance?.transfers[0];

  async function remove(e: Expense) {
    if (!window.confirm(t("money.deleteConfirm"))) return;
    await api.DELETE("/v1/households/{householdId}/expenses/{expenseId}", { params: { path: { householdId: household.id, expenseId: e.id } } }).catch(() => undefined);
    reload();
  }

  const back = (
    <Button variant="plain" size="sm" className="back" onClick={() => navigate("/")}>
      <span className="back-icon">
        <Icon name="chevron" size="sm" />
      </span>
      {t("tabs.today")}
    </Button>
  );
  return (
    <Screen title={t("money.title")} back={back} add={{ label: t("money.add"), onClick: () => setSheet("new") }}>
      {balance && (
        <Card
          eyebrow={<span>{t("money.balance")}</span>}
          title={standing(balance)}
          footer={
            transfer && (
              <Button size="sm" onClick={() => setSettling(transfer)}>
                {t("money.settle")}
              </Button>
            )
          }
        >
          {balance.transfers.length > 1 &&
            balance.transfers.map((x) => (
              <span key={`${x.from}-${x.to}`} className="money-transfer">
                {t("money.pays", { from: name(x.from), to: name(x.to), amount: money(x.cents) })}
              </span>
            ))}
        </Card>
      )}
      {failed && (
        <p className="callout error" role="alert">
          {t("money.failed")}
        </p>
      )}
      {months?.length === 1 && months[0]?.expenses.length === 0 && !earlier && <Empty title={t("money.emptyTitle")}>{t("money.emptyBody")}</Empty>}
      {months?.map((m) =>
        m.expenses.length === 0 ? null : (
          <section className="group" key={m.month}>
            <h2 className="label group-title">{monthName.format(new Date(`${m.month}-01T00:00`))}</h2>
            {m.total > 0 && <MonthSummary month={m} />}
            <div className="hl-list">
              {m.expenses.map((e) => (
                <ExpenseRow key={e.id} expense={e} household={household} me={me} onOpen={() => setSheet(e)} onDelete={() => remove(e)} />
              ))}
            </div>
          </section>
        ),
      )}
      {earlier && (
        <Button variant="secondary" block onClick={loadEarlier}>
          {t("money.earlier", { month: monthName.format(new Date(`${earlier}-01T00:00`)) })}
        </Button>
      )}
      {sheet && (
        <ExpenseSheet
          household={household}
          me={me}
          expense={sheet === "new" ? undefined : sheet}
          onClose={() => setSheet(null)}
          onSaved={() => {
            setSheet(null);
            reload();
          }}
        />
      )}
      {settling && (
        <SettleSheet
          household={household}
          me={me}
          transfer={settling}
          onClose={() => setSettling(null)}
          onSaved={() => {
            setSettling(null);
            reload();
          }}
        />
      )}
    </Screen>
  );
}

function ExpenseRow({ expense: e, household, me, onOpen, onDelete }: { expense: Expense; household: HouseholdDetail; me: User; onOpen: () => void; onDelete: () => void }) {
  const { t, i18n } = useTranslation();
  const money = useMoney();
  const i = household.members.findIndex((m) => m.user.id === e.paidBy);
  const payer = household.members[i]?.user;
  const who = (id: string) => (id === me.id ? t("money.you") : (household.members.find((m) => m.user.id === id)?.user.name ?? t("money.someone")));
  const day = new Intl.DateTimeFormat(i18n.language, { day: "numeric", month: "short" }).format(new Date(`${e.day}T00:00`));
  const settlement = e.kind === "settlement";
  const receiver = e.shares[0]?.userId;
  const mine = e.shares.find((s) => s.userId === me.id)?.cents ?? 0;
  const name = (id?: string) => household.members.find((m) => m.user.id === id)?.user.name ?? t("money.someone");
  const backTitle = e.paidBy === me.id ? t("money.paidBackByYou", { name: name(receiver) }) : receiver === me.id ? t("money.paidYouBack", { name: name(e.paidBy) }) : t("money.paidBack", { from: name(e.paidBy), to: name(receiver) });
  const title = settlement ? backTitle : e.note || t(`money.category.${e.category}`);
  const subtitle = settlement
    ? day
    : [day, t("money.paidBy", { name: who(e.paidBy) }), e.note && t(`money.category.${e.category}`)].filter(Boolean).join(" · ");
  return (
    <Swipe end={{ label: t("entries.delete"), icon: "trash", tone: "danger", onAction: onDelete }}>
      <ListRow
        leading={payer && <ThreadAvatar name={payer.name} src={payer.avatarUrl} thread={threadFor(i)} size="sm" />}
        title={title}
        subtitle={subtitle}
        onClick={settlement ? undefined : onOpen}
        trailing={
          <span className="money-amount">
            <span>{money(e.cents)}</span>
            {!settlement && mine > 0 && mine !== e.cents && <span className="caption">{t("money.yours", { amount: money(mine) })}</span>}
          </span>
        }
      />
    </Swipe>
  );
}

type Split = "even" | "custom";

// Add or change an expense: how much, what for, who paid, and how it's split: evenly, or each
// person's part typed in.
function ExpenseSheet({ household, me, expense, onClose, onSaved }: {
  household: HouseholdDetail;
  me: User;
  expense?: Expense;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t, i18n } = useTranslation();
  const money = useMoney();
  const members = household.members.map((m) => m.user);
  const [amount, setAmount] = useState(expense ? centsText(expense.cents, i18n.language) : "");
  const [note, setNote] = useState(expense?.note ?? "");
  const [category, setCategory] = useState<ExpenseCategory>(expense?.category ?? "groceries");
  const [paidBy, setPaidBy] = useState(expense?.paidBy ?? me.id);
  const [day, setDay] = useState(expense?.day ?? today());
  const evenNow = !expense || isEven(expense, members.map((m) => m.id));
  const [split, setSplit] = useState<Split>(evenNow ? "even" : "custom");
  const [parts, setParts] = useState<Record<string, string>>(() =>
    Object.fromEntries(members.map((m) => [m.id, expense ? centsText(expense.shares.find((s) => s.userId === m.id)?.cents ?? 0, i18n.language) : ""])),
  );
  // undefined keeps the receipt as it is, null removes it, a Blob replaces it.
  const [receipt, setReceipt] = useState<Blob | null | undefined>(undefined);
  const [preview, setPreview] = useState<string | undefined>(expense?.receiptUrl);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // An expense saved before its receipt failed is changed, not added again, on the next try.
  const saved = useRef<Expense | undefined>(expense);
  useEffect(() => () => {
    if (preview?.startsWith("blob:")) URL.revokeObjectURL(preview);
  }, [preview]);

  async function pick(file: File | undefined) {
    if (!file) return;
    setError(null);
    const small = await shrink(file).catch(() => undefined);
    if (!small) return setError(t("money.receiptFailed"));
    setReceipt(small);
    setPreview(URL.createObjectURL(small));
  }

  const cents = parseCents(amount);
  const partCents = members.map((m) => (parts[m.id]?.trim() ? parseCents(parts[m.id] ?? "") : 0));
  const left = cents - partCents.reduce((a, b) => a + b, 0);
  const ready = cents > 0 && note.length <= 200 && (split === "even" || (partCents.every((c) => !Number.isNaN(c)) && left === 0));

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!ready) return;
    setBusy(true);
    setError(null);
    const body = {
      cents,
      category,
      note: note.trim() || undefined,
      paidBy,
      day,
      shares: split === "custom" ? members.map((m, i) => ({ userId: m.id, cents: partCents[i] ?? 0 })) : undefined,
    };
    const current = saved.current;
    const res = await (current
      ? api.PUT("/v1/households/{householdId}/expenses/{expenseId}", { params: { path: { householdId: household.id, expenseId: current.id } }, body })
      : api.POST("/v1/households/{householdId}/expenses", { params: { path: { householdId: household.id } }, body })
    ).catch(() => undefined);
    if (!res?.data) {
      setBusy(false);
      setError(res?.error?.message ?? t("money.failed"));
      return;
    }
    saved.current = res.data;
    const path = { params: { path: { householdId: household.id, expenseId: res.data.id } } };
    let photo: { error?: { message: string } } | undefined = {};
    if (receipt) {
      photo = await api
        .PUT("/v1/households/{householdId}/expenses/{expenseId}/receipt", {
          ...path,
          // The contract types a binary body as a string; the photo goes as it is.
          body: receipt as unknown as string,
          bodySerializer: (b) => b,
          headers: { "Content-Type": "image/jpeg" },
        })
        .catch(() => undefined);
    } else if (receipt === null && res.data.receiptUrl) {
      photo = await api.DELETE("/v1/households/{householdId}/expenses/{expenseId}/receipt", path).catch(() => undefined);
    }
    setBusy(false);
    if (photo && !photo.error) onSaved();
    else setError(photo?.error?.message ?? t("money.receiptFailed"));
  }

  async function remove() {
    if (!expense || !window.confirm(t("money.deleteConfirm"))) return;
    setBusy(true);
    const res = await api.DELETE("/v1/households/{householdId}/expenses/{expenseId}", { params: { path: { householdId: household.id, expenseId: expense.id } } }).catch(() => undefined);
    setBusy(false);
    if (res?.response.ok) onSaved();
    else setError(res?.error?.message ?? t("money.failed"));
  }

  return (
    <Sheet
      title={t(expense ? "money.edit" : "money.new")}
      onClose={onClose}
      onSubmit={submit}
      leading={
        <Button variant="plain" onClick={onClose}>
          {t("entries.cancel")}
        </Button>
      }
      trailing={
        <Button type="submit" size="sm" disabled={busy || !ready}>
          {t("entries.save")}
        </Button>
      }
    >
      <label className="field">
        <span className="label">{t("money.amount")}</span>
        <input className="input money-input" inputMode="decimal" autoFocus={!expense} value={amount} onChange={(e) => setAmount(e.target.value)} placeholder={centsText(0, i18n.language)} required />
      </label>
      <label className="field">
        <span className="label">{t("money.note")}</span>
        <input className="input" value={note} maxLength={200} onChange={(e) => setNote(e.target.value)} placeholder={t("money.notePlaceholder")} />
      </label>

      <FormGroup>
        <FormBlock label={t("money.categoryLabel")}>
          <Pills label={t("money.categoryLabel")} value={category} options={categories.map((c) => ({ value: c, label: t(`money.category.${c}`) }))} onChange={setCategory} />
        </FormBlock>
        {members.length > 1 && (
          <FormBlock label={t("money.paidByLabel")}>
            <Segmented label={t("money.paidByLabel")} value={paidBy} options={members.map((m) => ({ value: m.id, label: m.id === me.id ? t("money.you") : m.name }))} onChange={setPaidBy} />
          </FormBlock>
        )}
        <DateRow label={t("money.day")} type="date" value={day} onChange={setDay} placeholder={t("entries.pick")} required />
      </FormGroup>

      {members.length > 1 && (
        <div className="field">
          <span className="label">{t("money.split")}</span>
          <Segmented label={t("money.split")} value={split} options={(["even", "custom"] as const).map((s) => ({ value: s, label: t(`money.splits.${s}`) }))} onChange={setSplit} />
          {split === "even" && cents > 0 && <p className="caption muted sheet-hint">{t("money.evenHint", { amount: money(Math.floor(cents / members.length)) })}</p>}
          {split === "custom" && (
            <FormGroup>
              {members.map((m) => (
                <label key={m.id} className="form-row">
                  <span className="form-row-label">{m.id === me.id ? t("money.you") : m.name}</span>
                  <input
                    className="input money-part"
                    inputMode="decimal"
                    value={parts[m.id] ?? ""}
                    placeholder={centsText(0, i18n.language)}
                    onChange={(e) => setParts((p) => ({ ...p, [m.id]: e.target.value }))}
                  />
                </label>
              ))}
            </FormGroup>
          )}
          {split === "custom" && cents > 0 && left !== 0 && !Number.isNaN(left) && (
            <p className="caption muted sheet-hint">{t(left > 0 ? "money.left" : "money.over", { amount: money(Math.abs(left)) })}</p>
          )}
        </div>
      )}

      {preview && (
        <a href={preview} target="_blank" rel="noreferrer">
          <img className="moment-photo" src={preview} alt={t("money.receipt")} />
        </a>
      )}
      <div className="moment-photo-actions">
        <label className="hl-btn hl-btn-secondary hl-btn-sm">
          <Icon name="camera" size="sm" />
          {t(preview ? "money.receiptChange" : "money.receiptAdd")}
          <input className="visually-hidden" type="file" accept="image/*" onChange={(e) => pick(e.target.files?.[0])} />
        </label>
        {preview && (
          <Button variant="plain" size="sm" onClick={() => (setReceipt(null), setPreview(undefined))}>
            {t("money.receiptRemove")}
          </Button>
        )}
      </div>

      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {expense && (
        <Button variant="danger" block disabled={busy} onClick={remove}>
          {t("entries.delete")}
        </Button>
      )}
    </Sheet>
  );
}

// What a month spent, and on what: a bar per category, most first.
function MonthSummary({ month }: { month: ExpenseMonth }) {
  const { t } = useTranslation();
  const money = useMoney();
  const top = month.categories[0]?.cents ?? 1;
  return (
    <div className="money-summary">
      <div className="money-summary-total">
        <span className="caption muted">{t("money.spent")}</span>
        <span className="title">{money(month.total)}</span>
      </div>
      <ul className="money-bars">
        {month.categories.map((c) => (
          <li key={c.category}>
            <span className="caption">{t(`money.category.${c.category}`)}</span>
            <span className="money-bar" aria-hidden="true">
              <span style={{ width: `${Math.max(2, (c.cents / top) * 100)}%` }} />
            </span>
            <span className="caption money-bar-amount">{money(c.cents)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

// Whether the shares are what an even split would give, so the sheet opens on "evenly".
function isEven(e: Expense, members: string[]) {
  const n = members.length;
  return members.every((id, i) => {
    const want = Math.floor(e.cents / n) + (i < e.cents % n ? 1 : 0);
    return (e.shares.find((s) => s.userId === id)?.cents ?? 0) === want;
  });
}

// Settle up: one person pays the other back, the suggested amount or less.
function SettleSheet({ household, me, transfer, onClose, onSaved }: { household: HouseholdDetail; me: User; transfer: Transfer; onClose: () => void; onSaved: () => void }) {
  const { t, i18n } = useTranslation();
  const [amount, setAmount] = useState(centsText(transfer.cents, i18n.language));
  const [day, setDay] = useState(today());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const name = (id: string) => household.members.find((m) => m.user.id === id)?.user.name ?? t("money.someone");
  const cents = parseCents(amount);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const res = await api
      .POST("/v1/households/{householdId}/settlements", { params: { path: { householdId: household.id } }, body: { from: transfer.from, to: transfer.to, cents, day } })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) onSaved();
    else setError(res?.error?.message ?? t("money.failed"));
  }

  return (
    <Sheet
      title={t("money.settle")}
      onClose={onClose}
      onSubmit={submit}
      leading={
        <Button variant="plain" onClick={onClose}>
          {t("entries.cancel")}
        </Button>
      }
      trailing={
        <Button type="submit" size="sm" disabled={busy || !(cents > 0)}>
          {t("money.settled")}
        </Button>
      }
    >
      <p className="callout">
        {transfer.from === me.id
          ? t("money.settleYouPay", { name: name(transfer.to) })
          : transfer.to === me.id
            ? t("money.settlePaysYou", { name: name(transfer.from) })
            : t("money.settleHint", { from: name(transfer.from), to: name(transfer.to) })}
      </p>
      <label className="field">
        <span className="label">{t("money.amount")}</span>
        <input className="input money-input" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} required />
      </label>
      <FormGroup>
        <DateRow label={t("money.day")} type="date" value={day} max={today()} onChange={setDay} placeholder={t("entries.pick")} required />
      </FormGroup>
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
    </Sheet>
  );
}
