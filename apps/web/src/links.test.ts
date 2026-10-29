import { readFileSync } from "node:fs";
import { afterEach, expect, test, vi } from "vitest";
import { takeLink } from "./links";

// A stand-in for the Cache API, which jsdom doesn't have.
function fakeCaches() {
  const stores = new Map<string, Map<string, Response>>();
  const open = async (name: string) => {
    const store = stores.get(name) ?? new Map<string, Response>();
    stores.set(name, store);
    return {
      put: async (key: string, res: Response) => void store.set(key, res),
      match: async (key: string) => store.get(key)?.clone(),
      delete: async (key: string) => store.delete(key),
    };
  };
  vi.stubGlobal("caches", { open });
  return { open };
}

afterEach(() => vi.unstubAllGlobals());

const keep = async (c: ReturnType<typeof fakeCaches>, link: unknown) =>
  (await c.open("homeloom-link")).put("/__homeloom/link", new Response(JSON.stringify(link)));

test("a kept link is taken once while it's fresh", async () => {
  const c = fakeCaches();
  await keep(c, { path: "/lists?entry=1", at: 1_000 });
  expect(await takeLink(30_000)).toBe("/lists?entry=1");
  expect(await takeLink(30_000)).toBeNull();
});

test("an old link or another site's is dropped", async () => {
  const c = fakeCaches();
  await keep(c, { path: "/us", at: 1_000 });
  expect(await takeLink(1_000 + 61_000)).toBeNull();
  await keep(c, { path: "//evil.example/", at: 1_000 });
  expect(await takeLink(2_000)).toBeNull();
});

// public/push-sw.js runs in the service worker; this runs it against a stand-in for one.
function serviceWorker(windows: { postMessage: ReturnType<typeof vi.fn>; focus: ReturnType<typeof vi.fn> }[]) {
  const c = fakeCaches();
  const listeners: Record<string, (e: unknown) => void> = {};
  const self = {
    location: new URL("https://home.example/sw.js"),
    addEventListener: (type: string, fn: (e: unknown) => void) => (listeners[type] = fn),
    clients: { matchAll: vi.fn(async () => windows), openWindow: vi.fn(async () => null) },
  };
  new Function("self", "caches", readFileSync("public/push-sw.js", "utf8"))(self, { open: c.open });
  const click = async (url?: string) => {
    let done: Promise<unknown> = Promise.resolve();
    listeners.notificationclick?.({ notification: { data: { url }, close: vi.fn() }, waitUntil: (p: Promise<unknown>) => (done = p) });
    await done;
  };
  return { self, click };
}

test("tapping a notification tells the open app where to go and brings it forward", async () => {
  const win = { postMessage: vi.fn(), focus: vi.fn(async () => win) };
  const sw = serviceWorker([win]);
  await sw.click("/call/abc?from=me");
  expect(win.postMessage).toHaveBeenCalledWith({ type: "homeloom:open", path: "/call/abc?from=me" });
  expect(win.focus).toHaveBeenCalled();
  expect(sw.self.clients.openWindow).not.toHaveBeenCalled();
  // Kept too, in case the app was just starting.
  expect(await takeLink()).toBe("/call/abc?from=me");
});

test("tapping a notification with the app closed opens it at the link, and only at this app", async () => {
  const sw = serviceWorker([]);
  await sw.click("/lists?entry=1");
  expect(sw.self.clients.openWindow).toHaveBeenCalledWith("/lists?entry=1");
  await sw.click("https://evil.example/x");
  expect(sw.self.clients.openWindow).toHaveBeenLastCalledWith("/");
});
