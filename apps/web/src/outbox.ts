import type { components } from "./api/schema.gen";
import type { Chore, ChoreCompletion, Entry, EntryKind, Expense, ExpenseMonth, Household, HouseholdDetail, ShoppingItem, User } from "./api/client";

type ExpenseShare = components["schemas"]["ExpenseShare"];
import { cached, keep } from "./cache";
import i18n from "./i18n";
import { splitNote } from "./notes";

// Changes made without a network wait here, on this device, and are sent in order once it is back.
// Until then they are applied to what the app reads, so lists show them at once. Something made
// offline gets a local id that later changes are sent against, and that turns into the real one.
//
// An edit of an entry carries the version it was made on (If-Match). If someone changed or deleted
// the entry in the meantime, their version stays and the edit is saved as a copy, so nothing is lost.
// Ticks, aisles and deletes need no such care: the last one wins.

type Op = {
  id: string;
  // Who made the change; it is never sent for someone else signed in later.
  user: string;
  household: string;
  method: string;
  path: string;
  body?: Record<string, unknown>;
  // The local id of what a create makes.
  local?: string;
  // An entry edit: the version it is based on, and its kind for a copy.
  ifMatch?: string;
  kind?: EntryKind;
  // A chore ticked off: what it was, to show in what was done until it is sent.
  chore?: { title: string; points: number };
  at: number;
};

export type OutboxState = { pending: number; syncing: boolean; copies: number };

const opsKey = "homeloom.outbox";
const idsKey = "homeloom.outbox.ids";
// After a failed try, the next one waits this long, unless the phone comes back online first.
const retryAfter = 30_000;

function read<T>(key: string, empty: T): T {
  try {
    const raw = localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : empty;
  } catch {
    return empty;
  }
}

let ops = read<Op[]>(opsKey, []);
// Local ids of things that have been made, and their real ids.
let ids = read<Record<string, string>>(idsKey, {});
let state: OutboxState = { pending: ops.length, syncing: false, copies: 0 };
const listeners = new Set<() => void>();
const syncedListeners = new Set<() => void>();

function save() {
  try {
    localStorage.setItem(opsKey, JSON.stringify(ops));
    localStorage.setItem(idsKey, JSON.stringify(ids));
  } catch {
    // Kept in memory until the app closes.
  }
  set({ pending: ops.length });
}

function set(next: Partial<OutboxState>) {
  state = { ...state, ...next };
  for (const l of listeners) l();
}

export function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => void listeners.delete(listener);
}

export function outboxState() {
  return state;
}

// onSynced runs after changes were sent, so screens can load again.
export function onSynced(listener: () => void) {
  syncedListeners.add(listener);
  return () => void syncedListeners.delete(listener);
}

// seenCopies says the copies made so far have been seen.
export function seenCopies() {
  set({ copies: 0 });
}

// clear drops every change not sent yet, when someone signs out.
export function clear() {
  ops = [];
  ids = {};
  save();
}

// The changes that wait offline, by method and path.
type Change =
  | { type: "createEntry"; household: string }
  | { type: "updateEntry" | "deleteEntry" | "done" | "undone"; entry: string }
  | { type: "addItem" | "clearDone"; household: string }
  | { type: "moveItem" | "removeItem" | "tick" | "untick"; household: string; item: string }
  | { type: "createExpense" | "settle" | "createChore"; household: string }
  | { type: "updateExpense" | "deleteExpense"; household: string; expense: string }
  | { type: "updateChore" | "deleteChore" | "choreDone"; household: string; chore: string }
  | { type: "undoCompletion"; household: string; completion: string };

// Kinds of change that make something, and so get a local id.
const creates = ["createEntry", "addItem", "createExpense", "settle", "createChore", "choreDone"];
const deletes = ["deleteEntry", "removeItem", "deleteExpense", "deleteChore", "undoCompletion"];

function change(method: string, path: string): Change | undefined {
  let m: RegExpExecArray | null;
  if ((m = /^\/v1\/households\/([^/]+)\/entries$/.exec(path)) && method === "POST") return { type: "createEntry", household: m[1] ?? "" };
  if ((m = /^\/v1\/entries\/([^/]+)$/.exec(path))) {
    if (method === "PUT") return { type: "updateEntry", entry: m[1] ?? "" };
    if (method === "DELETE") return { type: "deleteEntry", entry: m[1] ?? "" };
  }
  if ((m = /^\/v1\/entries\/([^/]+)\/done$/.exec(path))) {
    if (method === "PUT") return { type: "done", entry: m[1] ?? "" };
    if (method === "DELETE") return { type: "undone", entry: m[1] ?? "" };
  }
  if ((m = /^\/v1\/households\/([^/]+)\/shopping-list\/items$/.exec(path)) && method === "POST") return { type: "addItem", household: m[1] ?? "" };
  if ((m = /^\/v1\/households\/([^/]+)\/shopping-list\/done$/.exec(path)) && method === "DELETE") return { type: "clearDone", household: m[1] ?? "" };
  if ((m = /^\/v1\/households\/([^/]+)\/shopping-list\/items\/([^/]+)$/.exec(path))) {
    if (method === "PATCH") return { type: "moveItem", household: m[1] ?? "", item: m[2] ?? "" };
    if (method === "DELETE") return { type: "removeItem", household: m[1] ?? "", item: m[2] ?? "" };
  }
  if ((m = /^\/v1\/households\/([^/]+)\/shopping-list\/items\/([^/]+)\/done$/.exec(path))) {
    if (method === "PUT") return { type: "tick", household: m[1] ?? "", item: m[2] ?? "" };
    if (method === "DELETE") return { type: "untick", household: m[1] ?? "", item: m[2] ?? "" };
  }
  if ((m = /^\/v1\/households\/([^/]+)\/(expenses|settlements|chores)$/.exec(path)) && method === "POST") {
    return { type: m[2] === "expenses" ? "createExpense" : m[2] === "settlements" ? "settle" : "createChore", household: m[1] ?? "" };
  }
  if ((m = /^\/v1\/households\/([^/]+)\/expenses\/([^/]+)$/.exec(path))) {
    if (method === "PUT") return { type: "updateExpense", household: m[1] ?? "", expense: m[2] ?? "" };
    if (method === "DELETE") return { type: "deleteExpense", household: m[1] ?? "", expense: m[2] ?? "" };
  }
  if ((m = /^\/v1\/households\/([^/]+)\/chores\/([^/]+)$/.exec(path))) {
    if (method === "PUT") return { type: "updateChore", household: m[1] ?? "", chore: m[2] ?? "" };
    if (method === "DELETE") return { type: "deleteChore", household: m[1] ?? "", chore: m[2] ?? "" };
  }
  if ((m = /^\/v1\/households\/([^/]+)\/chores\/([^/]+)\/done$/.exec(path)) && method === "POST") return { type: "choreDone", household: m[1] ?? "", chore: m[2] ?? "" };
  if ((m = /^\/v1\/households\/([^/]+)\/chore-completions\/([^/]+)$/.exec(path)) && method === "DELETE") {
    return { type: "undoCompletion", household: m[1] ?? "", completion: m[2] ?? "" };
  }
  return undefined;
}

// What a change is about, local or real; for one that makes something, what it makes.
function about(c: Change) {
  if ("entry" in c) return c.entry;
  if ("item" in c) return c.item;
  if ("expense" in c) return c.expense;
  if (c.type === "choreDone") return undefined;
  if ("chore" in c) return c.chore;
  if ("completion" in c) return c.completion;
  return undefined;
}

function target(op: Op) {
  const c = change(op.method, op.path);
  return c && (about(c) ?? op.local);
}

const me = () => cached<User>("/v1/me");
const iso = (at: number) => new Date(at).toISOString();
// The phone's date, as the API writes days.
const day = (at: number) => {
  const d = new Date(at);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};

function madeEntry(op: Op): Entry {
  const body = op.body ?? {};
  const kind = body.kind as EntryKind;
  const text = String(body.text ?? "");
  return {
    ...(body as Partial<Entry>),
    id: op.local ?? "",
    kind,
    authorId: op.user,
    visibility: (body.visibility as Entry["visibility"]) ?? "shared",
    sealed: false,
    allDay: body.allDay === true,
    repeat: (body.repeat as Entry["repeat"]) ?? "never",
    ...(kind === "note" ? { title: splitNote(text).title } : {}),
    createdAt: iso(op.at),
    updatedAt: iso(op.at),
  };
}

function madeItem(op: Op): ShoppingItem {
  return { id: op.local ?? "", text: String(op.body?.text ?? ""), aisle: "other", addedBy: op.user, done: false, createdAt: iso(op.at) };
}

// An expense split evenly, as the API does without shares: the first members carry odd cents.
function evenShares(household: string, cents: number): ExpenseShare[] {
  const members = cached<HouseholdDetail>(`/v1/households/${household}`)?.members ?? [];
  const n = Math.max(members.length, 1);
  return members.map((m, i) => ({ userId: m.user.id, cents: Math.floor(cents / n) + (i < cents % n ? 1 : 0) }));
}

function madeExpense(op: Op, household: string): Expense {
  const body = op.body ?? {};
  const cents = Number(body.cents ?? 0);
  const at = { day: String(body.day ?? day(op.at)), addedBy: op.user, createdAt: iso(op.at), updatedAt: iso(op.at) };
  if (op.path.endsWith("/settlements")) {
    const to = String(body.to ?? "");
    return { id: op.local ?? "", kind: "settlement", cents, category: "other", paidBy: String(body.from ?? ""), shares: [{ userId: to, cents }], ...at };
  }
  return {
    ...(body as Partial<Expense>),
    id: op.local ?? "",
    kind: "expense",
    cents,
    category: (body.category as Expense["category"]) ?? "other",
    paidBy: String(body.paidBy ?? op.user),
    shares: (body.shares as ExpenseShare[] | undefined) ?? evenShares(household, cents),
    ...at,
  };
}

function madeChore(op: Op): Chore {
  const body = op.body ?? {};
  const rotate = body.rotate === true;
  return {
    ...(body as Partial<Chore>),
    id: op.local ?? "",
    title: String(body.title ?? ""),
    points: Number(body.points ?? 1),
    repeat: (body.repeat as Chore["repeat"]) ?? "never",
    rotate,
    assigneeId: (body.assigneeId as string | undefined) ?? (rotate ? op.user : undefined),
    dueOn: String(body.dueOn ?? day(op.at)),
    createdBy: op.user,
    createdAt: iso(op.at),
  };
}

function madeCompletion(op: Op, chore: string): ChoreCompletion {
  return { id: op.local ?? "", choreId: chore, title: op.chore?.title ?? "", points: op.chore?.points ?? 1, userId: op.user, day: day(op.at), doneAt: iso(op.at), thankedBy: [] };
}

const expenseFields = ["cents", "category", "note", "paidBy", "day", "shares"];
const choreFields = ["title", "points", "repeat", "rotate", "assigneeId", "dueOn"];

// edited is something with the fields an edit sends replaced: one it leaves out is cleared.
function edited<T extends object>(x: T, fields: string[], body?: Record<string, unknown>): T {
  return { ...(Object.fromEntries(Object.entries(x).filter(([k]) => !fields.includes(k))) as T), ...(body as Partial<T>) };
}

const entryFields = ["text", "visibility", "revealAt", "dueAt", "startsAt", "endsAt", "allDay", "alertMinutes", "repeat", "assigneeId"];

function onEntry(e: Entry, op: Op): Entry | undefined {
  const c = change(op.method, op.path);
  if (!c || !("entry" in c) || c.entry !== e.id) return e;
  switch (c.type) {
    case "deleteEntry":
      return undefined;
    case "done":
      return { ...e, doneAt: iso(op.at), doneBy: op.user };
    case "undone":
      return { ...e, doneAt: undefined, doneBy: undefined };
    case "updateEntry": {
      // An edit sends every field, so one it leaves out is cleared.
      const kept = Object.fromEntries(Object.entries(e).filter(([k]) => !entryFields.includes(k)));
      const text = String(op.body?.text ?? "");
      return { ...(kept as Entry), ...(op.body as Partial<Entry>), ...(e.kind === "note" ? { title: splitNote(text).title } : {}), updatedAt: iso(op.at) };
    }
  }
}

function onItem(it: ShoppingItem, op: Op): ShoppingItem | undefined {
  const c = change(op.method, op.path);
  if (c?.type === "clearDone") return it.done ? undefined : it;
  if (!c || !("item" in c) || c.item !== it.id) return it;
  switch (c.type) {
    case "removeItem":
      return undefined;
    case "tick":
      return { ...it, done: true, doneAt: iso(op.at) };
    case "untick":
      return { ...it, done: false, doneAt: undefined };
    case "moveItem":
      return { ...it, ...(op.body as Partial<ShoppingItem>) };
  }
}

const mine = () => ops.filter((op) => op.user === me()?.id);

// overlay applies the changes not sent yet to what a GET answered.
function overlay(path: string, search: string, value: unknown): unknown {
  const pending = mine();
  if (!pending.length) return value;
  let m: RegExpExecArray | null;
  if ((m = /^\/v1\/households\/([^/]+)\/entries$/.exec(path)) && Array.isArray(value)) {
    const kind = new URLSearchParams(search).get("kind");
    let list = value as Entry[];
    for (const op of pending) {
      const c = change(op.method, op.path);
      if (c?.type === "createEntry" && c.household === m[1]) {
        const e = madeEntry(op);
        if (!kind || e.kind === kind) list = [...list, e];
      } else list = list.flatMap((e) => onEntry(e, op) ?? []);
    }
    return list;
  }
  if (/^\/v1\/entries\/[^/]+$/.test(path) && value && typeof value === "object") {
    let e: Entry | undefined = value as Entry;
    for (const op of pending) e = e && onEntry(e, op);
    return e ?? value;
  }
  if ((m = /^\/v1\/households\/([^/]+)\/shopping-list$/.exec(path)) && Array.isArray(value)) {
    let list = value as ShoppingItem[];
    for (const op of pending) {
      const c = change(op.method, op.path);
      if (c?.type === "addItem" && c.household === m[1]) list = [...list, madeItem(op)];
      else if (c && "household" in c && c.household === m[1]) list = list.flatMap((it) => onItem(it, op) ?? []);
    }
    return list;
  }
  if ((m = /^\/v1\/households\/([^/]+)\/expenses$/.exec(path)) && value && typeof value === "object" && "expenses" in value) {
    const month = value as ExpenseMonth;
    let list = month.expenses;
    for (const op of pending) {
      const c = change(op.method, op.path);
      if (!c || !("household" in c) || c.household !== m[1]) continue;
      if (c.type === "createExpense" || c.type === "settle") {
        const e = madeExpense(op, c.household);
        if (e.day.startsWith(month.month)) list = [e, ...list];
      } else if (c.type === "updateExpense") list = list.map((e) => (e.id === c.expense ? { ...edited(e, expenseFields, op.body), updatedAt: iso(op.at) } : e));
      else if (c.type === "deleteExpense") list = list.filter((e) => e.id !== c.expense);
    }
    if (list === month.expenses) return value;
    list = [...list].sort((a, b) => b.day.localeCompare(a.day));
    // The total follows; the categories are the server's until it has the change.
    return { ...month, expenses: list, total: list.reduce((n, e) => n + (e.kind === "expense" ? e.cents : 0), 0) };
  }
  if ((m = /^\/v1\/households\/([^/]+)\/chores$/.exec(path)) && Array.isArray(value)) {
    let list = value as Chore[];
    for (const op of pending) {
      const c = change(op.method, op.path);
      if (!c || !("household" in c) || c.household !== m[1]) continue;
      if (c.type === "createChore") list = [...list, madeChore(op)];
      else if (c.type === "updateChore") list = list.map((x) => (x.id === c.chore ? edited(x, choreFields, op.body) : x));
      // Ticked off, it is gone until the server says when it is next due.
      else if (c.type === "deleteChore" || c.type === "choreDone") list = list.filter((x) => x.id !== c.chore);
    }
    return list;
  }
  if ((m = /^\/v1\/households\/([^/]+)\/chore-completions$/.exec(path)) && Array.isArray(value)) {
    let list = value as ChoreCompletion[];
    for (const op of pending) {
      const c = change(op.method, op.path);
      if (!c || !("household" in c) || c.household !== m[1]) continue;
      if (c.type === "choreDone") list = [madeCompletion(op, c.chore), ...list];
      else if (c.type === "undoCompletion") list = list.filter((d) => d.id !== c.completion);
    }
    return list;
  }
  return value;
}

// local is what this device has for a GET, with the changes not sent yet: for a screen's first render.
export function local<T>(path: string, query?: Record<string, string | undefined>): T | undefined {
  const value = cached<T>(path, query);
  if (value === undefined) return undefined;
  const search = new URLSearchParams(Object.entries(query ?? {}).filter((e): e is [string, string] => e[1] !== undefined)).toString();
  return overlay(path, search, value) as T;
}

// What something local is about, as the app sees it now.
function current(op: Op, id: string): Entry | ShoppingItem | Expense | Chore | undefined {
  const c = change(op.method, op.path);
  if (!c) return undefined;
  if ("item" in c) return local<ShoppingItem[]>(`/v1/households/${c.household}/shopping-list`)?.find((it) => it.id === id);
  if (c.type === "createExpense" || c.type === "settle" || "expense" in c) {
    return local<ExpenseMonth>(`/v1/households/${c.household}/expenses`)?.expenses.find((e) => e.id === id) ?? (op.local === id ? madeExpense(op, c.household) : undefined);
  }
  if (c.type === "createChore" || "chore" in c) return local<Chore[]>(`/v1/households/${c.household}/chores`)?.find((x) => x.id === id) ?? (op.local === id ? madeChore(op) : undefined);
  for (const kind of [undefined, "note", "reminder", "event"]) {
    const e = local<Entry[]>(`/v1/households/${op.household}/entries`, { kind })?.find((x) => x.id === id);
    if (e) return e;
  }
  return local<Entry>(`/v1/entries/${id}`);
}

// The version of an entry the server last sent, for If-Match.
function serverVersion(household: string, id: string) {
  for (const kind of [undefined, "note", "reminder", "event"]) {
    const e = cached<Entry[]>(`/v1/households/${household}/entries`, { kind })?.find((x) => x.id === id);
    if (e) return e.updatedAt;
  }
  return cached<Entry>(`/v1/entries/${id}`)?.updatedAt;
}

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

// enqueue keeps a change to send later and answers as the server would have.
function enqueue(method: string, path: string, body: Record<string, unknown> | undefined): Response {
  const user = me()?.id;
  const household = cached<Household[]>("/v1/households")?.[0]?.id;
  const c = change(method, path);
  if (!user || !household || !c) throw new TypeError("offline");
  const op: Op = { id: crypto.randomUUID(), user, household, method, path, body, at: Date.now() };
  const id = about(c);
  const made = id ? ops.find((o) => o.local === id) : undefined;

  if (creates.includes(c.type)) {
    op.local = crypto.randomUUID();
    if (c.type === "choreDone") {
      const chore = local<Chore[]>(`/v1/households/${c.household}/chores`)?.find((x) => x.id === c.chore);
      op.chore = { title: chore?.title ?? "", points: chore?.points ?? 1 };
    }
    ops = [...ops, op];
    save();
    if (c.type === "createEntry") return json(madeEntry(op), 201);
    if (c.type === "addItem") return json(madeItem(op), 201);
    if (c.type === "createChore") return json(madeChore(op), 201);
    if (c.type === "choreDone") return json(madeCompletion(op, c.chore), 201);
    return json(madeExpense(op, household), 201);
  }
  if (deletes.includes(c.type)) {
    // Something deleted before it was sent is never sent at all; earlier changes to it needn't be either.
    ops = ops.filter((o) => target(o) !== id);
    if (!made) ops = [...ops, op];
    save();
    return new Response(null, { status: 204 });
  }
  if (c.type === "updateEntry" || c.type === "updateExpense" || c.type === "updateChore") {
    const earlier = made ?? ops.find((o) => o.method === "PUT" && change(o.method, o.path)?.type === c.type && target(o) === id);
    if (earlier) {
      // The latest fields replace the earlier ones; what a create makes and the first edit's version stay.
      earlier.body = earlier.local && earlier.body?.kind ? { kind: earlier.body.kind, ...body } : body;
      save();
      return json(current(earlier, id ?? ""));
    }
    if (c.type === "updateEntry") {
      op.ifMatch = serverVersion(household, id ?? "");
      op.kind = (current(op, id ?? "") as Entry | undefined)?.kind;
    }
  }
  ops = [...ops, op];
  save();
  const now = id ? current(op, id) : undefined;
  return now ? json(now) : new Response(null, { status: 204 });
}

// The real path of something made offline once it has been made.
function resolve(path: string) {
  return path.replace(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/g, (id) => ids[id] ?? id);
}

// send is the client's fetch. GETs go through the cache with the changes not sent yet applied;
// changes go out at once when they can, and wait in the outbox when they can't.
export async function send(req: Request, fetcher: (req: Request) => Promise<Response>): Promise<Response> {
  const url = new URL(req.url);
  const path = resolve(url.pathname);
  if (path !== url.pathname) req = new Request(new URL(path + url.search, url), req);

  if (req.method === "GET") {
    // Something made offline and not sent yet is only here.
    const made = ops.find((o) => o.local && path.endsWith(`/${o.local}`));
    if (made && change(made.method, made.path)?.type === "createEntry") return json(overlay(path, "", madeEntry(made)));
    const res = await keep(req, fetcher);
    if (!res.ok || !mine().length || !res.headers.get("Content-Type")?.includes("json")) return res;
    const value = overlay(path, url.search, await res.clone().json());
    return json(value, res.status);
  }

  const c = change(req.method, path);
  if (!c) return keep(req, fetcher);
  const text = await req.clone().text();
  const body = text ? (JSON.parse(text) as Record<string, unknown>) : undefined;
  // Something made offline can only change offline until it is made; others go behind what waits.
  const id = about(c);
  const waiting = mine().length > 0 || (!!id && !!ops.find((o) => o.local === id));
  if (!waiting && navigator.onLine) {
    try {
      return await keep(req, fetcher);
    } catch {
      // No network after all: it waits.
    }
  }
  const res = enqueue(req.method, path, body);
  void drain(fetcher);
  return res;
}

let draining = false;
let retry: ReturnType<typeof setTimeout> | undefined;

function request(op: Op) {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (op.ifMatch) headers["If-Match"] = `"${op.ifMatch}"`;
  return new Request(new URL(op.path, window.location.origin), {
    method: op.method,
    headers,
    body: op.body ? JSON.stringify(op.body) : undefined,
    credentials: "same-origin",
  });
}

// copyOf is an entry edit that couldn't be made, as a new entry of its own.
function copyOf(op: Op): Op {
  const text = String(op.body?.text ?? "");
  const [first = "", ...rest] = text.split("\n");
  const marked = [`${first} ${i18n.t("offline.copy")}`, ...rest].join("\n");
  return {
    ...op,
    id: crypto.randomUUID(),
    method: "POST",
    path: `/v1/households/${op.household}/entries`,
    body: { ...op.body, text: marked, kind: op.kind ?? "note" },
    ifMatch: undefined,
  };
}

// drain sends what waits, oldest first, and stops at the first change the server can't take now.
export async function drain(fetcher: (req: Request) => Promise<Response> = (r) => globalThis.fetch(r)) {
  if (draining || !navigator.onLine || !ops.length) return;
  draining = true;
  clearTimeout(retry);
  set({ syncing: true });
  let sent = 0;
  try {
    while (ops[0]) {
      const op = ops[0];
      const user = me()?.id;
      // Signed out: it waits for the same person to sign in again.
      if (!user) break;
      if (op.user !== user) {
        ops = ops.slice(1);
        save();
        continue;
      }
      const res = await fetcher(request(op)).catch(() => undefined);
      if (!res || res.status >= 500 || res.status === 401 || res.status === 429) {
        if (res?.status !== 401) retry = setTimeout(() => void drain(fetcher), retryAfter);
        break;
      }
      const c = change(op.method, op.path);
      let next = ops.slice(1);
      if (res.ok) {
        const data = res.status === 204 ? undefined : ((await res.json().catch(() => undefined)) as { id?: string; updatedAt?: string } | undefined);
        if (op.local && data?.id) {
          ids[op.local] = data.id;
          const local = op.local;
          next = next.map((o) => ({ ...o, path: o.path.replace(local, data.id ?? local) }));
        }
        // Later edits of this entry are based on the version this change made.
        const id = data?.id;
        if (id && data.updatedAt) next = next.map((o) => (o.ifMatch && target(o) === id ? { ...o, ifMatch: data.updatedAt } : o));
      } else if (c?.type === "updateEntry" && (res.status === 412 || res.status === 404)) {
        // Someone changed or deleted it meanwhile: theirs stays, this edit becomes a copy.
        next = [copyOf(op), ...next];
        set({ copies: state.copies + 1 });
      }
      // Any other refusal (gone, not allowed) can't be sent again.
      ops = next;
      sent++;
      save();
    }
  } finally {
    draining = false;
    set({ syncing: false });
    if (sent) for (const l of syncedListeners) l();
  }
}
