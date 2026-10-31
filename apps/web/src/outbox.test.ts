import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { keep } from "./cache";
import { drain, outboxState, send } from "./outbox";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const origin = window.location.origin;
const req = (method: string, path: string, body?: unknown) =>
  new Request(origin + path, { method, body: body === undefined ? undefined : JSON.stringify(body), headers: { "Content-Type": "application/json" } });
const down = () => Promise.reject(new TypeError("Load failed"));

const h = "33333333-3333-4333-8333-333333333333";
const me = { id: "11111111-1111-4111-8111-111111111111", name: "Marius" };
const note = { id: "44444444-4444-4444-8444-444444444444", kind: "note", authorId: me.id, visibility: "shared", sealed: false, allDay: false, repeat: "never", text: "# Trip\n\nPack", title: "Trip", createdAt: "2026-10-29T08:00:00.000001Z", updatedAt: "2026-10-29T08:00:00.000001Z" };

let online = true;
beforeEach(async () => {
  online = true;
  vi.spyOn(navigator, "onLine", "get").mockImplementation(() => online);
  await keep(req("GET", "/v1/me"), async () => json(me));
  await keep(req("GET", "/v1/households"), async () => json([{ id: h, name: "Our place", role: "owner" }]));
});
afterEach(() => vi.restoreAllMocks());

// A server that records what it was sent and answers with the given handler.
function server(handle: (r: Request) => Response | Promise<Response>) {
  const sent: { method: string; path: string; ifMatch: string | null; body: unknown }[] = [];
  const fetcher = vi.fn(async (r: Request) => {
    const text = await r.clone().text();
    sent.push({ method: r.method, path: new URL(r.url).pathname, ifMatch: r.headers.get("If-Match"), body: text ? JSON.parse(text) : undefined });
    return handle(r);
  });
  return { sent, fetcher };
}

test("shopping changes made offline show at once and go out in order once back", async () => {
  const list = `/v1/households/${h}/shopping-list`;
  await keep(req("GET", list), async () => json([]));
  online = false;

  const added = await send(req("POST", `${list}/items`, { text: "Milk" }), down);
  expect(added.status).toBe(201);
  const item = (await added.json()) as { id: string };
  await send(req("PUT", `${list}/items/${item.id}/done`), down);
  const shown = (await (await send(req("GET", list), down)).json()) as { text: string; done: boolean }[];
  expect(shown).toMatchObject([{ text: "Milk", done: true }]);
  expect(outboxState().pending).toBe(2);

  online = true;
  const { sent, fetcher } = server((r) => (r.method === "POST" ? json({ id: "55555555-5555-4555-8555-555555555555", text: "Milk" }, 201) : json({})));
  await drain(fetcher);
  expect(sent.map((s) => `${s.method} ${s.path}`)).toEqual([`POST ${list}/items`, `PUT ${list}/items/55555555-5555-4555-8555-555555555555/done`]);
  expect(outboxState().pending).toBe(0);
});

test("a note made and written offline is made once, with what it says last", async () => {
  online = false;
  const made = (await (await send(req("POST", `/v1/households/${h}/entries`, { kind: "note", text: "# Ideas", visibility: "shared" }), down)).json()) as { id: string };
  await send(req("PUT", `/v1/entries/${made.id}`, { text: "# Ideas\n\nA boat", visibility: "shared" }), down);
  const read = (await (await send(req("GET", `/v1/entries/${made.id}`), down)).json()) as { title: string; text: string };
  expect(read).toMatchObject({ title: "Ideas", text: "# Ideas\n\nA boat" });

  online = true;
  const real = "66666666-6666-4666-8666-666666666666";
  const { sent, fetcher } = server(() => json({ ...note, id: real, text: "# Ideas\n\nA boat" }, 201));
  await drain(fetcher);
  expect(sent).toEqual([{ method: "POST", path: `/v1/households/${h}/entries`, ifMatch: null, body: { kind: "note", text: "# Ideas\n\nA boat", visibility: "shared" } }]);

  // The page still has the local id; what it sends now goes to the real note.
  await send(req("PUT", `/v1/entries/${made.id}`, { text: "# Ideas", visibility: "shared" }), fetcher);
  expect(sent[1]?.path).toBe(`/v1/entries/${real}`);
});

test("an offline edit of a note someone changed meanwhile is kept as a copy", async () => {
  await keep(req("GET", `/v1/households/${h}/entries?kind=note`), async () => json([note]));
  online = false;
  await send(req("PUT", `/v1/entries/${note.id}`, { text: "# Trip\n\nPack the tent", visibility: "shared" }), down);
  await send(req("PUT", `/v1/entries/${note.id}`, { text: "# Trip\n\nPack the tent and stove", visibility: "shared" }), down);
  const shown = (await (await send(req("GET", `/v1/households/${h}/entries?kind=note`), down)).json()) as { text: string }[];
  expect(shown).toMatchObject([{ text: "# Trip\n\nPack the tent and stove" }]);

  online = true;
  const { sent, fetcher } = server((r) => (r.method === "PUT" ? json({ code: "entry_changed", message: "" }, 412) : json({ ...note, id: "77777777-7777-4777-8777-777777777777" }, 201)));
  await drain(fetcher);
  expect(sent).toEqual([
    { method: "PUT", path: `/v1/entries/${note.id}`, ifMatch: `"${note.updatedAt}"`, body: { text: "# Trip\n\nPack the tent and stove", visibility: "shared" } },
    { method: "POST", path: `/v1/households/${h}/entries`, ifMatch: null, body: { kind: "note", text: "# Trip (offline copy)\n\nPack the tent and stove", visibility: "shared" } },
  ]);
  expect(outboxState()).toMatchObject({ pending: 0, copies: 1 });
});

test("something made and deleted offline is never sent", async () => {
  const list = `/v1/households/${h}/shopping-list`;
  online = false;
  const item = (await (await send(req("POST", `${list}/items`, { text: "Eggs" }), down)).json()) as { id: string };
  expect((await send(req("DELETE", `${list}/items/${item.id}`), down)).status).toBe(204);
  expect(outboxState().pending).toBe(0);
});

test("with the API down, changes wait and are tried again", async () => {
  vi.useFakeTimers();
  online = false;
  await send(req("PUT", `/v1/entries/${note.id}/done`), down);
  online = true;
  let up = false;
  const { sent, fetcher } = server(() => (up ? json(note) : json({ code: "unavailable" }, 503)));
  await drain(fetcher);
  expect(outboxState().pending).toBe(1);
  up = true;
  await vi.advanceTimersByTimeAsync(30_000);
  expect(sent).toHaveLength(2);
  expect(outboxState().pending).toBe(0);
  vi.useRealTimers();
});

test("changes someone else made on this device are never sent as you", async () => {
  online = false;
  await send(req("DELETE", `/v1/entries/${note.id}`), down);
  await keep(req("GET", "/v1/me"), async () => json({ id: "22222222-2222-4222-8222-222222222222", name: "Jo" }));
  online = true;
  const { sent, fetcher } = server(() => json({}));
  await drain(fetcher);
  expect(sent).toHaveLength(0);
});
