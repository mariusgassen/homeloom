import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { App } from "./App";
import i18n from "./i18n";
import { SessionProvider } from "./session";

// A stand-in for LiveKit's Room: connects at once and lets a test play the other side.
const rooms = vi.hoisted(() => [] as InstanceType<typeof import("node:events").EventEmitter>[]);
vi.mock("livekit-client", async () => {
  const { EventEmitter } = await import("node:events");
  class Room extends EventEmitter {
    remoteParticipants = new Map<string, { identity: string; name: string; isAgent: boolean; getTrackPublication: () => undefined }>();
    canPlaybackAudio = true;
    localParticipant = {
      isMicrophoneEnabled: false,
      isCameraEnabled: false,
      getTrackPublication: () => undefined,
      setMicrophoneEnabled: vi.fn(async (on: boolean) => {
        this.localParticipant.isMicrophoneEnabled = on;
      }),
      setCameraEnabled: vi.fn(),
    };
    connect = vi.fn(async () => undefined);
    disconnect = vi.fn(async () => undefined);
    startAudio = vi.fn(async () => undefined);
    constructor() {
      super();
      rooms.push(this);
    }
    join(identity: string, name: string, isAgent = false) {
      this.remoteParticipants.set(identity, { identity, name, isAgent, getTrackPublication: () => undefined });
      this.emit("participantConnected");
    }
  }
  return {
    Room,
    RoomEvent: Object.fromEntries(
      ["ParticipantConnected", "ParticipantDisconnected", "Reconnecting", "Reconnected", "Disconnected", "TrackSubscribed", "TrackUnsubscribed", "TrackMuted", "TrackUnmuted", "LocalTrackPublished", "LocalTrackUnpublished", "AudioPlaybackStatusChanged"].map((e) => [
        e,
        e.charAt(0).toLowerCase() + e.slice(1),
      ]),
    ),
    Track: { Source: { Camera: "camera", Microphone: "microphone" }, Kind: { Video: "video", Audio: "audio" } },
  };
});

type Route = (req: Request) => Response | undefined;

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

const marius = { id: "11111111-1111-4111-8111-111111111111", name: "Marius" };
const jo = { id: "22222222-2222-4222-8222-222222222222", name: "Jo" };
const home = { id: "33333333-3333-4333-8333-333333333333", name: "Our place", role: "owner" };

function serve(...routes: Route[]) {
  const calls: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      calls.push(req);
      for (const r of routes) {
        const res = r(req);
        if (res) return res;
      }
      return json({ code: "not_found", message: "Not found" }, 404);
    }),
  );
  return calls;
}

const at = (method: string, path: string, res: () => Response): Route => (req) =>
  req.method === method && new URL(req.url).pathname === path ? res() : undefined;

function renderApp(path = "/") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <SessionProvider>
        <App />
      </SessionProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => i18n.changeLanguage("en"));
afterEach(() => {
  vi.unstubAllGlobals();
  delete (navigator as { serviceWorker?: unknown }).serviceWorker;
});

test("signed out shows the sign-in", async () => {
  serve(at("GET", "/v1/me", () => json({ code: "unauthenticated", message: "" }, 401)));
  renderApp();
  expect(await screen.findByRole("button", { name: "Sign in" })).toBeInTheDocument();
});

test("API down shows a retry, not the sign-in", async () => {
  serve(at("GET", "/v1/me", () => json({ code: "auth_unavailable", message: "" }, 503)));
  renderApp();
  expect(await screen.findByRole("button", { name: "Try again" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Sign in" })).not.toBeInTheDocument();
});

test("offline says so instead of blaming the server", async () => {
  vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
  serve();
  vi.mocked(fetch).mockRejectedValue(new TypeError("Load failed"));
  renderApp();
  expect(await screen.findByRole("heading", { name: "You're offline" })).toBeInTheDocument();
  vi.restoreAllMocks();
});

test("signed in with a household shows the tabs and both people", async () => {
  serve(
    at("GET", "/v1/me", () => json(marius)),
    at("GET", "/v1/households", () => json([home])),
    at("GET", `/v1/households/${home.id}`, () =>
      json({ ...home, members: [{ user: marius, role: "owner" }, { user: jo, role: "member" }] }),
    ),
  );
  renderApp();
  expect(await screen.findByRole("heading", { level: 1, name: "Today" })).toBeInTheDocument();
  const nav = screen.getByRole("navigation", { name: "Main" });
  expect(nav).toHaveTextContent("TodayCookUsListsHome");
  expect(screen.getByRole("link", { name: "Today" })).toHaveAttribute("aria-current", "page");
  expect(screen.getByRole("img", { name: "Jo" })).toHaveClass("hl-avatar-berry");
});

test("German follows the language", async () => {
  await i18n.changeLanguage("de");
  serve(at("GET", "/v1/me", () => json({}, 401)));
  renderApp();
  expect(await screen.findByRole("button", { name: "Anmelden" })).toBeInTheDocument();
});

test("without a household, onboarding creates one", async () => {
  let created = false;
  const calls = serve(
    at("GET", "/v1/me", () => json(marius)),
    at("GET", "/v1/households", () => json(created ? [home] : [])),
    at("POST", "/v1/households", () => {
      created = true;
      return json(home, 201);
    }),
    at("GET", `/v1/households/${home.id}`, () => json({ ...home, members: [{ user: marius, role: "owner" }] })),
  );
  renderApp();
  await userEvent.type(await screen.findByLabelText("Household name"), "  Our place ");
  await userEvent.click(screen.getByRole("button", { name: "Create household" }));
  await waitFor(() => expect(screen.getByRole("heading", { level: 1, name: "Today" })).toBeInTheDocument());
  const post = calls.find((c) => c.method === "POST");
  expect(await post?.json()).toEqual({ name: "Our place" });
});

const pair = { ...home, members: [{ user: marius, role: "owner" }, { user: jo, role: "member" }] };
const signedIn = (detail: unknown = pair): Route[] => [
  at("GET", "/v1/me", () => json(marius)),
  at("GET", "/v1/households", () => json([home])),
  at("GET", `/v1/households/${home.id}`, () => json(detail)),
];

test("a question stays sealed until both have answered", async () => {
  const q = { id: "44444444-4444-4444-8444-444444444444", text: "Best trip?", askedBy: jo.id, createdAt: "2026-10-27T18:00:00Z", revealed: false };
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/questions`, () => json([{ ...q, answers: [{ userId: jo.id, answeredAt: q.createdAt }] }])),
    at("PUT", `/v1/questions/${q.id}/answer`, () =>
      json({
        ...q,
        revealed: true,
        revealedAt: "2026-10-28T08:00:00Z",
        answers: [
          { userId: jo.id, text: "Lisbon", answeredAt: q.createdAt },
          { userId: marius.id, text: "Porto", answeredAt: "2026-10-28T08:00:00Z" },
        ],
      }),
    ),
  );
  renderApp("/us");
  expect(await screen.findByText("Jo has answered")).toBeInTheDocument();
  expect(screen.queryByText("Lisbon")).not.toBeInTheDocument();
  await userEvent.type(screen.getByLabelText("Your answer"), "Porto");
  await userEvent.click(screen.getByRole("button", { name: "Answer" }));
  expect(await screen.findByText("Lisbon")).toBeInTheDocument();
  expect(screen.getByText("Porto")).toBeInTheDocument();
  expect(await calls.find((c) => c.method === "PUT")?.json()).toEqual({ text: "Porto" });
});

test("settings creates an invite link", async () => {
  serve(...signedIn({ ...home, members: [{ user: marius, role: "owner" }] }), at("POST", `/v1/households/${home.id}/invites`, () => json({ code: "abc", expiresAt: "2026-11-04T10:00:00Z" }, 201)));
  renderApp("/settings");
  await userEvent.click(await screen.findByRole("button", { name: "Invite someone" }));
  expect(await screen.findByText(`${window.location.origin}/join/abc`)).toBeInTheDocument();
});

// A browser that can do Web Push, with a push service that hands out one fixed subscription.
function stubPush(permission: NotificationPermission) {
  let current: PushSubscription | null = null;
  const sub = {
    endpoint: "https://web.push.apple.com/abc",
    toJSON: () => ({ endpoint: "https://web.push.apple.com/abc", keys: { p256dh: "p", auth: "a" } }),
    unsubscribe: vi.fn(async () => ((current = null), true)),
  } as unknown as PushSubscription;
  const pushManager = {
    getSubscription: async () => current,
    subscribe: vi.fn(async () => (current = sub)),
  };
  vi.stubGlobal("PushManager", function PushManager() {});
  vi.stubGlobal("Notification", { permission, requestPermission: vi.fn(async () => permission) });
  Object.defineProperty(navigator, "serviceWorker", { configurable: true, value: { ready: Promise.resolve({ pushManager }) } });
  return pushManager;
}

test("settings turns on notifications for this device", async () => {
  const pushManager = stubPush("granted");
  const calls = serve(
    ...signedIn(),
    at("GET", "/v1/push", () => json({ publicKey: "AQID" })),
    at("POST", "/v1/push/subscriptions", () => new Response(null, { status: 204 })),
    at("DELETE", "/v1/push/subscriptions", () => new Response(null, { status: 204 })),
  );
  renderApp("/settings");
  const toggle = await screen.findByRole("switch", { name: /Notifications on this device/ });
  expect(toggle).toHaveAttribute("aria-checked", "false");
  await userEvent.click(toggle);
  await waitFor(() => expect(toggle).toHaveAttribute("aria-checked", "true"));
  expect(pushManager.subscribe).toHaveBeenCalledWith({ userVisibleOnly: true, applicationServerKey: new Uint8Array([1, 2, 3]) });
  const post = calls.find((c) => c.method === "POST");
  expect(post?.headers.get("Accept-Language")).toBe("en");
  expect(await post?.json()).toEqual({ endpoint: "https://web.push.apple.com/abc", keys: { p256dh: "p", auth: "a" } });

  await userEvent.click(toggle);
  await waitFor(() => expect(toggle).toHaveAttribute("aria-checked", "false"));
  const del = calls.find((c) => c.method === "DELETE");
  expect(new URL(del?.url ?? "/", window.location.origin).searchParams.get("endpoint")).toBe("https://web.push.apple.com/abc");
});

test("settings says when notifications are blocked", async () => {
  stubPush("denied");
  serve(...signedIn(), at("GET", "/v1/push", () => json({ publicKey: "AQID" })));
  renderApp("/settings");
  await userEvent.click(await screen.findByRole("switch", { name: /Notifications on this device/ }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Notifications are blocked.");
});

test("settings hides notifications when the server has none", async () => {
  stubPush("default");
  serve(...signedIn(), at("GET", "/v1/push", () => json({ code: "push_unavailable", message: "" }, 503)));
  renderApp("/settings");
  expect(await screen.findByRole("button", { name: "Invite someone" })).toBeInTheDocument();
  expect(screen.queryByRole("switch")).not.toBeInTheDocument();
});

test("an invite link waits through sign-in, then joins", async () => {
  serve(at("GET", "/v1/me", () => json({}, 401)));
  const { unmount } = renderApp("/join/abc");
  expect(await screen.findByText(/Sign in to join/)).toBeInTheDocument();
  unmount();
  vi.unstubAllGlobals();

  // Back from authentik on "/": the stored code is accepted before onboarding would show.
  let joined = false;
  serve(
    at("GET", "/v1/me", () => json(marius)),
    at("GET", "/v1/households", () => json(joined ? [home] : [])),
    at("POST", "/v1/invites/abc", () => {
      joined = true;
      return json(home);
    }),
    at("GET", `/v1/households/${home.id}`, () => json(pair)),
  );
  renderApp("/");
  expect(await screen.findByRole("heading", { level: 1, name: "Today" })).toBeInTheDocument();
  expect(joined).toBe(true);
  expect(localStorage.getItem("homeloom.invite")).toBeNull();
});

test("a used invite says so and lets you go on", async () => {
  serve(...signedIn(), at("POST", "/v1/invites/old", () => json({ code: "invite_invalid", message: "This invite link is no longer valid." }, 404)));
  renderApp("/join/old");
  expect(await screen.findByRole("alert")).toHaveTextContent("no longer valid");
  await userEvent.click(screen.getByRole("button", { name: "Continue" }));
  expect(await screen.findByRole("heading", { level: 1, name: "Today" })).toBeInTheDocument();
});

test("cook by mood suggests recipes and puts what's missing on the list", async () => {
  sessionStorage.clear();
  const curry = { slug: "green-curry", name: "Green curry", tags: ["Thai"], totalTime: "30 min" };
  const calls = serve(
    ...signedIn(),
    at("POST", `/v1/households/${home.id}/cook/suggestions`, () =>
      json({ chef: true, suggestions: [{ recipe: curry, reason: "Warm and a little spicy." }] }),
    ),
    at("GET", `/v1/households/${home.id}/recipes/green-curry`, () =>
      json({ ...curry, steps: ["Simmer."], ingredients: [{ id: "i1", text: "Coconut milk" }, { id: "i2", text: "Rice" }, { id: "i3", text: "Lime" }] }),
    ),
    at("POST", `/v1/households/${home.id}/shopping-list`, () => json({ added: 2, list: "Shopping list" })),
  );
  renderApp("/cook");
  await userEvent.click(await screen.findByRole("button", { name: "Cozy" }));
  await userEvent.click(screen.getByRole("button", { name: "20 min" }));
  await userEvent.type(screen.getByLabelText("Use up"), "lime");
  await userEvent.click(screen.getByRole("button", { name: "Suggest dinner" }));
  expect(await calls.find((c) => c.url.endsWith("/cook/suggestions"))?.json()).toEqual({ mood: "Cozy; Time: 20 min; Use up: lime" });

  await userEvent.click(await screen.findByText("Warm and a little spicy."));
  await userEvent.click(await screen.findByLabelText("Rice"));
  await userEvent.click(screen.getByRole("button", { name: "Put 2 missing items on the list" }));
  expect(await screen.findByRole("status")).toHaveTextContent("2 items are on “Shopping list” now.");
  expect(await calls.find((c) => c.url.endsWith("/shopping-list"))?.json()).toEqual({ recipe: "green-curry", ingredientIds: ["i1", "i3"] });
});

test("cooking says so when Mealie is down", async () => {
  sessionStorage.clear();
  serve(
    ...signedIn(),
    at("POST", `/v1/households/${home.id}/cook/suggestions`, () => json({ code: "cooking_unavailable", message: "Cooking isn't available right now." }, 503)),
  );
  renderApp("/cook");
  await userEvent.click(await screen.findByRole("button", { name: "Light" }));
  await userEvent.click(screen.getByRole("button", { name: "Suggest dinner" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Cooking isn't available");
});

test("home switches live scenes and lights, room by room", async () => {
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/home`, () =>
      json({
        connected: true,
        rooms: [{ id: "kitchen", name: "Kitchen" }],
        scenes: [{ id: "scene.dinner", name: "Abendessen", room: "kitchen" }, { id: "scene.movie", name: "Movie night" }],
        lights: [
          { id: "light.kitchen", name: "Kitchen", room: "kitchen", on: false },
          { id: "light.hall", name: "Hall", on: true },
        ],
        readings: [
          { id: "sensor.kitchen_temperature", name: "Kitchen temperature", room: "kitchen", kind: "temperature", value: 21.46, unit: "°C" },
          { id: "binary_sensor.kitchen_window", name: "Kitchen window", room: "kitchen", kind: "window", on: true },
          { id: "binary_sensor.hall_door", name: "Hall door", kind: "door", on: false },
        ],
      }),
    ),
    at("POST", `/v1/households/${home.id}/home/scenes/scene.movie`, () => new Response(null, { status: 204 })),
    at("POST", `/v1/households/${home.id}/home/lights/light.kitchen/toggle`, () => json({ id: "light.kitchen", name: "Kitchen", on: true })),
  );
  renderApp("/home");
  const kitchen = await screen.findByRole("region", { name: "Kitchen" });
  expect(kitchen).toHaveTextContent("21.5 °C");
  expect(kitchen).toHaveTextContent("Window open");
  expect(screen.getByRole("region", { name: "Other" })).not.toHaveTextContent("Door open");
  expect(within(kitchen).getByRole("button", { name: /Abendessen/ })).toBeInTheDocument();
  await userEvent.click(await screen.findByRole("button", { name: "Movie night" }));
  await waitFor(() => expect(screen.getByRole("button", { name: /Movie night/ })).toHaveAttribute("aria-pressed", "true"));

  const light = screen.getByRole("switch", { name: "Kitchen" });
  expect(light).toHaveAttribute("aria-checked", "false");
  await userEvent.click(light);
  await waitFor(() => expect(screen.getByRole("switch", { name: "Kitchen" })).toHaveAttribute("aria-checked", "true"));
  expect(calls.filter((c) => c.method === "POST").map((c) => new URL(c.url).pathname.split("/").slice(4).join("/"))).toEqual([
    "home/scenes/scene.movie",
    "home/lights/light.kitchen/toggle",
  ]);
});

test("home asks you to connect your own Home Assistant", async () => {
  const assign = vi.fn();
  vi.stubGlobal("location", { ...window.location, assign });
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/home`, () => json({ connected: false, rooms: [], scenes: [], lights: [], readings: [] })),
    at("POST", `/v1/households/${home.id}/home/connection`, () => json({ url: "https://ha.example/auth/authorize?state=s" })),
  );
  renderApp("/home?home_error=connect_failed");
  expect(await screen.findByRole("alert")).toHaveTextContent("Connecting didn't work.");
  await userEvent.click(await screen.findByRole("button", { name: "Connect" }));
  await waitFor(() => expect(assign).toHaveBeenCalledWith("https://ha.example/auth/authorize?state=s"));
  expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/home/connection"))).toBe(true);
});

test("home says so when Home Assistant isn't set up", async () => {
  serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/home`, () => json({ code: "home_unavailable", message: "The smart home isn't reachable right now." }, 503)),
  );
  renderApp("/home");
  expect(await screen.findByText("The smart home isn't reachable right now.")).toBeInTheDocument();
  expect(screen.getByText("Smart home isn't available")).toBeInTheDocument();
});

const entry = (fields: Record<string, unknown>) => ({
  id: "55555555-5555-4555-8555-555555555555",
  kind: "reminder",
  authorId: marius.id,
  visibility: "shared",
  sealed: false,
  allDay: false,
  repeat: "never",
  createdAt: "2026-10-27T18:00:00Z",
  updatedAt: "2026-10-27T18:00:00Z",
  ...fields,
});

test("reminders are ticked off in Lists", async () => {
  const bins = entry({ text: "Bins out", assigneeId: jo.id });
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([bins])),
    at("PUT", `/v1/entries/${bins.id}/done`, () => json({ ...bins, doneAt: "2026-10-28T08:00:00Z", doneBy: marius.id })),
  );
  renderApp("/lists");
  expect(await screen.findByRole("tab", { name: "Reminders" })).toHaveAttribute("aria-selected", "true");
  expect(await screen.findByText("Jo")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("checkbox", { name: "Bins out" }));
  await waitFor(() => expect(calls.some((c) => c.method === "PUT" && c.url.endsWith("/done"))).toBe(true));
  expect(calls.filter((c) => c.method === "GET" && new URL(c.url).pathname.endsWith("/entries"))).toHaveLength(2);
  expect(calls.find((c) => c.url.includes("/entries"))?.url).toContain("kind=reminder");
});

test("pulling down from the top loads the list again", async () => {
  const bins = entry({ text: "Bins out" });
  const calls = serve(...signedIn(), at("GET", `/v1/households/${home.id}/entries`, () => json([bins])));
  renderApp("/lists");
  const title = await screen.findByText("Bins out");
  const loads = () => calls.filter((c) => c.method === "GET" && new URL(c.url).pathname.endsWith("/entries")).length;
  expect(loads()).toBe(1);
  fireEvent.touchStart(title, { touches: [{ clientX: 100, clientY: 100 }] });
  fireEvent.touchMove(title, { touches: [{ clientX: 100, clientY: 140 }] });
  fireEvent.touchEnd(title);
  // A short pull isn't enough.
  await new Promise((r) => setTimeout(r, 100));
  expect(loads()).toBe(1);
  fireEvent.touchStart(title, { touches: [{ clientX: 100, clientY: 100 }] });
  fireEvent.touchMove(title, { touches: [{ clientX: 100, clientY: 260 }] });
  fireEvent.touchEnd(title);
  expect(await screen.findByRole("status")).toHaveTextContent("Loading…");
  await waitFor(() => expect(loads()).toBe(2));
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  expect(screen.getByText("Bins out")).toBeInTheDocument();
});

test("the shopping list adds, ticks and clears items", async () => {
  const list = `/v1/households/${home.id}/shopping-list`;
  const milk = { id: "44444444-4444-4444-8444-444444444444", text: "Milk", done: false, createdAt: "2026-10-28T08:00:00Z" };
  const lime = { id: "55555555-5555-4555-8555-555555555555", text: "Lime", recipe: "Green curry", done: true, createdAt: "2026-10-28T08:00:00Z" };
  const calls = serve(
    ...signedIn(),
    at("GET", list, () => json([milk, lime])),
    at("POST", `${list}/items`, () => json({ ...milk, text: "Bread" }, 201)),
    at("PUT", `${list}/items/${milk.id}/done`, () => json({ ...milk, done: true })),
    at("DELETE", `${list}/done`, () => new Response(null, { status: 204 })),
  );
  renderApp("/lists?show=shopping");
  expect(await screen.findByText("Green curry")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Add" })).toBeInTheDocument();
  expect(calls.some((c) => c.url.includes("/entries"))).toBe(false);

  await userEvent.type(screen.getByLabelText("Add an item"), "Bread");
  await userEvent.click(screen.getByRole("button", { name: "Add" }));
  await waitFor(() => expect(calls.some((c) => c.method === "POST")).toBe(true));
  expect(await calls.find((c) => c.method === "POST")?.json()).toEqual({ text: "Bread" });
  await waitFor(() => expect(screen.getByLabelText("Add an item")).toHaveValue(""));

  await userEvent.click(screen.getByRole("checkbox", { name: "Milk" }));
  await waitFor(() => expect(calls.some((c) => c.method === "PUT")).toBe(true));
  await userEvent.click(screen.getByRole("button", { name: "Clear" }));
  await waitFor(() => expect(calls.some((c) => c.method === "DELETE" && c.url.endsWith("/done"))).toBe(true));
});

test("the quick-add sheet sets a shared reminder for both", async () => {
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([])),
    at("POST", `/v1/households/${home.id}/entries`, () => json(entry({ text: "Call the landlord" }), 201)),
  );
  renderApp("/lists");
  expect(await screen.findByText("Nothing to remember")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Add" }));
  const sheet = screen.getByRole("dialog", { name: "New reminder" });
  await userEvent.type(within(sheet).getByLabelText("Remind about"), "Call the landlord");
  expect(within(sheet).getByRole("radio", { name: /Both of us/ })).toBeChecked();
  expect(within(sheet).getByText("Jo sees it right away.")).toBeInTheDocument();
  await userEvent.click(within(sheet).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(await calls.find((c) => c.method === "POST")?.json()).toEqual({ kind: "reminder", text: "Call the landlord", visibility: "shared", repeat: "never" });
});

test("a note for later stays sealed", async () => {
  serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () =>
      json([entry({ kind: "note", authorId: jo.id, visibility: "reveal", sealed: true, revealAt: "2030-02-14T00:00:00Z" })]),
    ),
  );
  renderApp("/lists?show=note");
  expect(await screen.findByText(/Jo wrote you something\. It opens on/)).toBeInTheDocument();
  expect(screen.getByRole("tab", { name: "Notes" })).toHaveAttribute("aria-selected", "true");
});

test("today lists what's due and today's events", async () => {
  const now = new Date();
  const soon = new Date(now.getTime() + 60_000).toISOString();
  serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () =>
      json([
        entry({ id: "66666666-6666-4666-8666-666666666666", kind: "event", text: "Dinner with Jo", startsAt: soon }),
        entry({ text: "Water the plants", dueAt: soon }),
        entry({ id: "77777777-7777-4777-8777-777777777777", text: "Next year", dueAt: "2099-01-01T08:00:00Z" }),
      ]),
    ),
  );
  renderApp("/");
  expect(await screen.findByText("Water the plants")).toBeInTheDocument();
  expect(screen.getByText("Dinner with Jo")).toBeInTheDocument();
  expect(screen.queryByText("Next year")).not.toBeInTheDocument();
});

test("settings makes a calendar subscription link", async () => {
  serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/calendar-feed`, () => json({ active: false })),
    at("POST", `/v1/households/${home.id}/calendar-feed`, () => json({ path: "/v1/calendar-feeds/secret" }, 201)),
  );
  renderApp("/settings");
  await userEvent.click(await screen.findByRole("button", { name: "Add to Calendar" }));
  expect(await screen.findByRole("link", { name: "Open in Calendar" })).toHaveAttribute("href", `webcal://${window.location.host}/v1/calendar-feeds/secret`);
  expect(screen.getByRole("button", { name: "Turn off" })).toBeInTheDocument();
});

const household = () =>
  json({ ...home, members: [{ user: marius, role: "owner" }, { user: jo, role: "member" }] });
const callId = "55555555-5555-4555-8555-555555555555";
const callJoin = () => json({ id: callId, url: "wss://home.example", token: "t", room: `call-${home.id}-${callId}` });
type FakeRoom = { connect: ReturnType<typeof vi.fn>; disconnect: ReturnType<typeof vi.fn>; join: (id: string, name: string, isAgent?: boolean) => void };

test("calling from Us rings and shows the call until hung up", async () => {
  rooms.length = 0;
  serve(
    at("GET", "/v1/me", () => json(marius)),
    at("GET", "/v1/households", () => json([home])),
    at("GET", `/v1/households/${home.id}`, household),
    at("GET", `/v1/households/${home.id}/questions`, () => json([])),
    at("POST", `/v1/households/${home.id}/calls`, callJoin),
    at("POST", `/v1/households/${home.id}/calls/${callId}/guest-links`, () => json({ token: "guesttoken", expiresAt: "2030-01-01T00:00:00Z" }, 201)),
  );
  renderApp("/us");
  await userEvent.click(await screen.findByRole("button", { name: "Call Jo" }));
  expect(await screen.findByText("Calling…")).toBeInTheDocument();
  const room = rooms[0] as unknown as FakeRoom;
  expect(room.connect).toHaveBeenCalledWith("wss://home.example", "t");

  act(() => room.join(jo.id, "Jo"));
  expect(await screen.findByText("0:00")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Mute" })).toHaveAttribute("aria-pressed", "false");

  // A guest link is made on one tap and shared on the next.
  await userEvent.click(screen.getByRole("button", { name: "Add people" }));
  const sheet = screen.getByRole("dialog", { name: "Add people" });
  await userEvent.click(within(sheet).getByRole("button", { name: "Create a guest link" }));
  expect(await within(sheet).findByText(`${window.location.origin}/guest/guesttoken`)).toBeInTheDocument();
  await userEvent.click(within(sheet).getByRole("button", { name: "Done" }));

  await userEvent.click(screen.getByRole("button", { name: "Hang up" }));
  expect(room.disconnect).toHaveBeenCalled();
  expect(screen.queryByText("0:00")).not.toBeInTheDocument();
});

test("a call notification asks before answering", async () => {
  rooms.length = 0;
  const calls = serve(
    at("GET", "/v1/me", () => json(marius)),
    at("GET", "/v1/households", () => json([home])),
    at("GET", `/v1/households/${home.id}`, household),
    at("GET", `/v1/households/${home.id}/questions`, () => json([])),
    at("POST", `/v1/households/${home.id}/calls/${callId}/join`, callJoin),
  );
  renderApp(`/call/${callId}?from=${jo.id}`);
  expect(await screen.findByRole("heading", { name: "Jo is calling" })).toBeInTheDocument();
  expect(rooms).toHaveLength(0);
  await userEvent.click(screen.getByRole("button", { name: "Answer" }));
  await waitFor(() => expect(rooms).toHaveLength(1));
  expect(calls.some((c) => c.url.endsWith(`/calls/${callId}/join`))).toBe(true);
});

test("with more than one other person, you pick who to call", async () => {
  rooms.length = 0;
  const sam = { id: "44444444-4444-4444-8444-444444444444", name: "Sam" };
  const calls = serve(
    at("GET", "/v1/me", () => json(marius)),
    at("GET", "/v1/households", () => json([home])),
    at("GET", `/v1/households/${home.id}`, () =>
      json({ ...home, members: [{ user: marius, role: "owner" }, { user: jo, role: "member" }, { user: sam, role: "member" }] }),
    ),
    at("GET", `/v1/households/${home.id}/questions`, () => json([])),
    at("POST", `/v1/households/${home.id}/calls`, callJoin),
  );
  renderApp("/us");
  await userEvent.click(await screen.findByRole("button", { name: "Who to call" }));
  const sheet = screen.getByRole("dialog", { name: "Who to call" });
  await userEvent.click(within(sheet).getByRole("checkbox", { name: /Sam/ }));
  await userEvent.click(within(sheet).getByRole("checkbox", { name: /Jo/ }));
  await userEvent.click(within(sheet).getByRole("button", { name: "Call" }));
  await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/calls"))).toBe(true));
  expect(await calls.find((c) => c.method === "POST" && c.url.endsWith("/calls"))?.json()).toEqual({ invite: [sam.id, jo.id] });
});

test("a guest joins with a name, without signing in", async () => {
  rooms.length = 0;
  const calls = serve(
    at("GET", "/v1/me", () => json({ code: "unauthenticated", message: "" }, 401)),
    at("GET", "/v1/guest-calls/guesttoken", () => json({ invitedBy: "Marius" })),
    at("POST", "/v1/guest-calls/guesttoken", callJoin),
  );
  renderApp("/guest/guesttoken");
  expect(await screen.findByRole("heading", { name: "Marius invites you to a call" })).toBeInTheDocument();
  await userEvent.type(screen.getByLabelText("Your name"), "Grandma");
  await userEvent.click(screen.getByRole("button", { name: "Join" }));
  await waitFor(() => expect(rooms).toHaveLength(1));
  const body = await calls.find((c) => c.method === "POST")?.json();
  expect(body.name).toBe("Grandma");
  expect(body.device).toBe(localStorage.getItem("homeloom.guest.guesttoken"));
  expect(screen.queryByRole("button", { name: "Add people" })).not.toBeInTheDocument();
});

test("a guest link used on another device says so", async () => {
  rooms.length = 0;
  serve(
    at("GET", "/v1/me", () => json({}, 401)),
    at("GET", "/v1/guest-calls/usedtoken", () => json({ invitedBy: "Marius" })),
    at("POST", "/v1/guest-calls/usedtoken", () => json({ code: "guest_link_used", message: "" }, 409)),
  );
  renderApp("/guest/usedtoken");
  await userEvent.type(await screen.findByLabelText("Your name"), "Mallory");
  await userEvent.click(screen.getByRole("button", { name: "Join" }));
  expect(await screen.findByText("This link is already in use on another device.")).toBeInTheDocument();
});

test("an expired guest link says so", async () => {
  serve(at("GET", "/v1/me", () => json({}, 401)));
  renderApp("/guest/oldtoken");
  expect(await screen.findByRole("alert")).toHaveTextContent("This link has expired");
});

test("settings makes and removes a key for another app", async () => {
  const token = { id: "66666666-6666-4666-8666-666666666666", name: "Claude", access: "tools", createdAt: "2026-10-28T12:00:00Z" };
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/mcp-tokens`, () => json([])),
    at("POST", `/v1/households/${home.id}/mcp-tokens`, () => json({ ...token, secret: "hlp_secret" }, 201)),
    at("DELETE", `/v1/mcp-tokens/${token.id}`, () => new Response(null, { status: 204 })),
  );
  renderApp("/settings");
  await userEvent.click(await screen.findByRole("tab", { name: "Assistant" }));
  await userEvent.type(screen.getByRole("textbox", { name: "App" }), "Claude");
  await userEvent.click(screen.getByRole("button", { name: "Make key" }));
  expect(await screen.findByText("hlp_secret")).toBeInTheDocument();
  expect(screen.getByText(`${window.location.origin}/mcp`)).toBeInTheDocument();
  const post = calls.find((c) => c.method === "POST" && c.url.endsWith("/mcp-tokens"));
  expect(await post?.json()).toEqual({ name: "Claude", access: "tools" });

  await userEvent.click(screen.getByRole("button", { name: "Done" }));
  expect(screen.queryByText("hlp_secret")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Remove" }));
  await waitFor(() => expect(screen.queryByText("Claude")).not.toBeInTheDocument());
});

test("settings makes a password for the iPhone's calendar", async () => {
  const token = { id: "77777777-7777-4777-8777-777777777777", name: "iPhone", access: "calendar", createdAt: "2026-10-28T12:00:00Z" };
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/mcp-tokens`, () => json([])),
    at("POST", `/v1/households/${home.id}/mcp-tokens`, () => json({ ...token, secret: "hlc_secret" }, 201)),
  );
  renderApp("/settings");
  await userEvent.type(await screen.findByRole("textbox", { name: "App" }), "iPhone");
  // "Add to iPhone" posts the form itself and gets a profile back.
  expect(screen.getByRole("button", { name: "Add to iPhone" }).closest("form")).toHaveAttribute("action", `/v1/households/${home.id}/calendar-profile`);
  await userEvent.click(screen.getByRole("button", { name: "Make password" }));
  expect(await screen.findByText("hlc_secret")).toBeInTheDocument();
  expect(screen.getByText(window.location.host)).toBeInTheDocument();
  expect(screen.getByText(/^Calendar · Made/)).toBeInTheDocument();
  const post = calls.find((c) => c.method === "POST" && c.url.endsWith("/mcp-tokens"));
  expect(await post?.json()).toEqual({ name: "iPhone", access: "calendar" });
});

test("settings makes a password for a notes app", async () => {
  const token = { id: "88888888-8888-4888-8888-888888888888", name: "iPhone", access: "notes", createdAt: "2026-10-28T12:00:00Z" };
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/mcp-tokens`, () => json([])),
    at("POST", `/v1/households/${home.id}/mcp-tokens`, () => json({ ...token, secret: "hln_secret" }, 201)),
  );
  renderApp("/settings");
  await userEvent.click(await screen.findByRole("tab", { name: "Notes" }));
  await userEvent.type(screen.getByRole("textbox", { name: "App" }), "iPhone");
  await userEvent.click(screen.getByRole("button", { name: "Make password" }));
  expect(await screen.findByText("hln_secret")).toBeInTheDocument();
  expect(screen.getByText(`${window.location.origin}/webdav/`)).toBeInTheDocument();
  expect(screen.getByText(/^Notes · Made/)).toBeInTheDocument();
  const post = calls.find((c) => c.method === "POST" && c.url.endsWith("/mcp-tokens"));
  expect(await post?.json()).toEqual({ name: "iPhone", access: "notes" });
});

const sse = (...events: [string, unknown][]) =>
  new Response(events.map(([e, d]) => `event: ${e}\ndata: ${JSON.stringify(d)}\n\n`).join(""), { headers: { "Content-Type": "text/event-stream" } });

test("the assistant answers in a chat and the screen loads again after it changed something", async () => {
  const conv = { id: "55555555-5555-4555-8555-555555555555", title: "Remind us", createdAt: "2026-10-28T12:00:00Z", updatedAt: "2026-10-28T12:00:00Z" };
  let entryLoads = 0;
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => {
      entryLoads++;
      return json([]);
    }),
    at("GET", `/v1/households/${home.id}/conversations`, () => json([])),
    at("POST", `/v1/households/${home.id}/chat`, () =>
      sse(
        ["conversation", conv],
        ["tool", { name: "add_reminder" }],
        ["text", { text: "Done, " }],
        ["text", { text: "it's on the list." }],
        ["done", { id: "66666666-6666-4666-8666-666666666666", role: "assistant", text: "Done, it's on the list.", createdAt: conv.createdAt }],
      ),
    ),
  );
  renderApp();
  await userEvent.click(await screen.findByRole("button", { name: "Ask Homeloom" }));
  const dialog = await screen.findByRole("dialog", { name: "Ask Homeloom" });
  await userEvent.type(within(dialog).getByLabelText("Message"), "Remind us to water the plants tomorrow at 8");
  await userEvent.click(within(dialog).getByRole("button", { name: "Send" }));
  expect(await within(dialog).findByText("Done, it's on the list.")).toBeInTheDocument();
  expect(within(dialog).getByText("Remind us to water the plants tomorrow at 8")).toHaveClass("bubble-me");
  const post = calls.find((c) => c.method === "POST");
  expect(await post?.json()).toEqual({ text: "Remind us to water the plants tomorrow at 8" });

  const before = entryLoads;
  await userEvent.click(within(dialog).getByRole("button", { name: "Done" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await waitFor(() => expect(entryLoads).toBeGreaterThan(before));
});

test("the assistant continues a recent chat and says when it isn't set up", async () => {
  const conv = { id: "55555555-5555-4555-8555-555555555555", title: "Was kochen wir?", createdAt: "2026-10-28T12:00:00Z", updatedAt: "2026-10-28T12:00:00Z" };
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/conversations`, () => json([conv])),
    at("GET", `/v1/conversations/${conv.id}`, () =>
      json({ ...conv, messages: [{ id: "a", role: "user", text: "Was kochen wir?", createdAt: conv.createdAt }, { id: "b", role: "assistant", text: "Curry!", createdAt: conv.createdAt }] }),
    ),
    at("POST", `/v1/households/${home.id}/chat`, () => json({ code: "assistant_unavailable", message: "The assistant isn't available right now." }, 503)),
  );
  renderApp();
  await userEvent.click(await screen.findByRole("button", { name: "Ask Homeloom" }));
  const dialog = await screen.findByRole("dialog");
  await userEvent.click(await within(dialog).findByRole("button", { name: "Was kochen wir?" }));
  expect(await within(dialog).findByText("Curry!")).toBeInTheDocument();
  await userEvent.type(within(dialog).getByLabelText("Message"), "Und morgen?");
  await userEvent.click(within(dialog).getByRole("button", { name: "Send" }));
  expect(await within(dialog).findByRole("alert")).toHaveTextContent("The assistant isn't available right now.");
  expect(await calls.find((c) => c.method === "POST")?.json()).toEqual({ text: "Und morgen?", conversationId: conv.id });
});

test("the phone in the assistant starts a call with Homeloom", async () => {
  rooms.length = 0;
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/conversations`, () => json([])),
    at("POST", `/v1/households/${home.id}/calls/homeloom`, () => json({ id: callId, url: "wss://home.example", token: "t", room: "homeloom-x" }, 201)),
  );
  renderApp();
  await userEvent.click(await screen.findByRole("button", { name: "Ask Homeloom" }));
  await userEvent.click(await screen.findByRole("button", { name: "Talk to Homeloom" }));
  expect(await screen.findByRole("heading", { name: "Homeloom" })).toBeInTheDocument();
  await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/calls/homeloom"))).toBe(true));

  const room = rooms[0] as unknown as FakeRoom;
  act(() => room.join("agent-1", "", true));
  expect(await screen.findByText("0:00")).toBeInTheDocument();
  expect(screen.getAllByText("Homeloom").length).toBeGreaterThan(0);
  // Nobody else can join a call with Homeloom.
  expect(screen.queryByRole("button", { name: "Add people" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Hang up" }));
  expect(room.disconnect).toHaveBeenCalled();
});

test("without the assistant the chat says so", async () => {
  serve(...signedIn(), at("GET", `/v1/households/${home.id}/conversations`, () => json({ code: "assistant_unavailable", message: "" }, 503)));
  renderApp();
  await userEvent.click(await screen.findByRole("button", { name: "Ask Homeloom" }));
  expect(await screen.findByText("Not available")).toBeInTheDocument();
  expect(screen.getByLabelText("Message")).toBeDisabled();
  expect(screen.queryByRole("button", { name: "Talk to Homeloom" })).not.toBeInTheDocument();
});
