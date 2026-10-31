import { expect, test } from "vitest";
import { cached, cachedKey, keep } from "./cache";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const get = (path: string) => new Request(`http://homeloom.test${path}`);
const down = () => Promise.reject(new TypeError("Load failed"));

test("keys follow the path and the query the client sends", () => {
  expect(cachedKey("/v1/households/h/entries", { kind: "note" })).toBe("/v1/households/h/entries?kind=note");
  expect(cachedKey("/v1/households/h/entries", { kind: undefined })).toBe("/v1/households/h/entries");
});

test("keeps what the app opens with and answers from it when the network or the API is down", async () => {
  await keep(get("/v1/households/h/entries?kind=note"), async () => json([{ id: "n1" }]));
  expect(cached("/v1/households/h/entries", { kind: "note" })).toEqual([{ id: "n1" }]);

  const offline = await keep(get("/v1/households/h/entries?kind=note"), down);
  expect(await offline.json()).toEqual([{ id: "n1" }]);
  const unavailable = await keep(get("/v1/households/h/entries?kind=note"), async () => json({ code: "unavailable" }, 503));
  expect(await unavailable.json()).toEqual([{ id: "n1" }]);

  // A refusal is the server's answer, not something to cover up.
  const missing = await keep(get("/v1/households/h/entries?kind=note"), async () => json({ code: "not_found" }, 404));
  expect(missing.status).toBe(404);
});

test("live states and changes are never kept", async () => {
  await keep(get("/v1/households/h/home"), async () => json({ rooms: [] }));
  expect(cached("/v1/households/h/home")).toBeUndefined();
  await expect(keep(get("/v1/households/h/home"), down)).rejects.toThrow("Load failed");

  await keep(new Request("http://homeloom.test/v1/households/h/entries", { method: "POST" }), async () => json({ id: "e1" }, 201));
  expect(cached("/v1/households/h/entries")).toBeUndefined();
});

test("another person signing in, or the session ending, drops everything", async () => {
  await keep(get("/v1/me"), async () => json({ id: "u1" }));
  await keep(get("/v1/households/h/shopping-list"), async () => json([{ id: "i1" }]));
  await keep(get("/v1/me"), async () => json({ id: "u2" }));
  expect(cached("/v1/households/h/shopping-list")).toBeUndefined();
  expect(cached("/v1/me")).toEqual({ id: "u2" });

  await keep(get("/v1/households/h/questions"), async () => json([]));
  await keep(get("/v1/households/h/questions"), async () => json({ code: "unauthenticated" }, 401));
  expect(cached("/v1/me")).toBeUndefined();
  expect(cached("/v1/households/h/questions")).toBeUndefined();
});
