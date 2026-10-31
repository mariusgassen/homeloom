import "fake-indexeddb/auto";
import { expect, test, vi } from "vitest";

const json = (body: unknown) => new Response(JSON.stringify(body), { headers: { "Content-Type": "application/json" } });
const get = (path: string) => new Request(`http://homeloom.test${path}`);

// Each import is the app opening again, with only what the device kept.
async function opened() {
  vi.resetModules();
  const cache = await import("./cache");
  await cache.preload();
  return cache;
}

test("what was kept is there when the app opens again, until someone signs out", async () => {
  const first = await opened();
  await first.keep(get("/v1/households/h/shopping-list"), async () => json([{ id: "i1" }]));

  const second = await opened();
  expect(second.cached("/v1/households/h/shopping-list")).toEqual([{ id: "i1" }]);
  second.forget();

  const third = await opened();
  expect(third.cached("/v1/households/h/shopping-list")).toBeUndefined();
});

test("answers older than a month are dropped when the app opens", async () => {
  const first = await opened();
  const now = Date.now();
  vi.spyOn(Date, "now").mockReturnValue(now - 31 * 24 * 60 * 60 * 1000);
  await first.keep(get("/v1/households/h/questions"), async () => json([]));
  vi.restoreAllMocks();
  await first.keep(get("/v1/households/h/entries"), async () => json([]));

  const second = await opened();
  expect(second.cached("/v1/households/h/questions")).toBeUndefined();
  expect(second.cached("/v1/households/h/entries")).toEqual([]);
});
