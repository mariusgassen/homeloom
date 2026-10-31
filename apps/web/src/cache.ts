// The last answer to each GET the app needs to open without a network, kept on this device: screens
// show it at once and load again behind it, and offline (or with the API down) the client answers
// from it. Live states such as the smart home are never kept. Everything is dropped when someone
// signs out, the session ends, or another person signs in on this device.

const kept = [
  /^\/v1\/me$/,
  /^\/v1\/households$/,
  /^\/v1\/households\/[^/]+$/,
  /^\/v1\/households\/[^/]+\/(entries|shopping-list|moments|moments\/resurfaced|questions|expenses|balance|chores|chore-completions|chore-fairness)$/,
  /^\/v1\/households\/[^/]+\/recipes\/[^/]+$/,
  /^\/v1\/entries\/[^/]+$/,
];
// Answers older than this aren't worth showing; they are dropped when the app opens.
const maxAge = 30 * 24 * 60 * 60 * 1000;
// The service worker keeps pictures here (vite.config.ts); they go with the rest.
export const picturesCache = "homeloom-pictures";

type Row = { key: string; body: string; at: number };
const memory = new Map<string, Row & { value?: unknown }>();
const store = "responses";
let db: Promise<IDBDatabase | undefined> | undefined;

function open() {
  db ??= new Promise((resolve) => {
    if (typeof indexedDB === "undefined") return resolve(undefined);
    const req = indexedDB.open("homeloom-cache", 1);
    req.onupgradeneeded = () => req.result.createObjectStore(store, { keyPath: "key" });
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => resolve(undefined);
  });
  return db;
}

// Writes are best effort: a phone without room still works from memory until it closes.
async function write(change: (s: IDBObjectStore) => void) {
  const d = await open();
  try {
    if (d) change(d.transaction(store, "readwrite").objectStore(store));
  } catch {
    // Not kept on the device this time.
  }
}

// preload reads what this device kept into memory, so the first render can use it. It gives up
// after a moment rather than hold up the app.
export async function preload(wait = 500) {
  const read = open().then(
    (d) =>
      new Promise<void>((resolve) => {
        if (!d) return resolve();
        const req = d.transaction(store).objectStore(store).getAll();
        req.onsuccess = () => {
          const old = Date.now() - maxAge;
          for (const row of req.result as Row[]) {
            if (row.at < old) void write((s) => s.delete(row.key));
            else if (!memory.has(row.key)) memory.set(row.key, row);
          }
          resolve();
        };
        req.onerror = () => resolve();
      }),
  );
  await Promise.race([read, new Promise((r) => setTimeout(r, wait))]);
}

// cachedKey is how a request is kept: its path and query, as the client sends them.
export function cachedKey(path: string, query?: Record<string, string | undefined>) {
  const q = new URLSearchParams(Object.entries(query ?? {}).filter((e): e is [string, string] => e[1] !== undefined)).toString();
  return q ? `${path}?${q}` : path;
}

// cached is the last answer kept for a path, or undefined. It is the same object every time.
export function cached<T>(path: string, query?: Record<string, string | undefined>): T | undefined {
  const row = memory.get(cachedKey(path, query));
  if (!row) return undefined;
  try {
    row.value ??= JSON.parse(row.body);
  } catch {
    return undefined;
  }
  return row.value as T;
}

function remember(key: string, body: string) {
  // Another person on this device: nothing of the one before stays.
  if (key === "/v1/me") {
    const before = cached<{ id?: string }>(key)?.id;
    try {
      if (before && before !== (JSON.parse(body) as { id?: string }).id) forget();
    } catch {
      return;
    }
  }
  const row = { key, body, at: Date.now() };
  memory.set(key, row);
  void write((s) => s.put(row));
}

// forget drops everything kept, on this device too.
export function forget() {
  memory.clear();
  void write((s) => s.clear());
  if (typeof caches !== "undefined") void caches.delete(picturesCache).catch(() => undefined);
}

// keep is the client's fetch: it keeps answers worth keeping, and answers from them when the network
// or the API is down.
export async function keep(req: Request, fetcher: (req: Request) => Promise<Response>) {
  const url = new URL(req.url);
  const key = url.pathname + url.search;
  const keepable = req.method === "GET" && kept.some((r) => r.test(url.pathname));
  const fallback = (reason: unknown) => {
    const row = keepable && memory.get(key);
    if (!row) throw reason;
    return new Response(row.body, { status: 200, headers: { "Content-Type": "application/json" } });
  };
  let res: Response;
  try {
    res = await fetcher(req);
  } catch (err) {
    return fallback(err);
  }
  if (res.status === 401) forget();
  else if (keepable && res.status >= 500 && memory.has(key)) return fallback(res);
  else if (keepable && res.ok) remember(key, await res.clone().text());
  return res;
}
