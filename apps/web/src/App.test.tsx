import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { App } from "./App";
import i18n from "./i18n";
import { nextDays } from "./meals";
import { SessionProvider } from "./session";

// A stand-in for LiveKit's Room: connects at once and lets a test play the other side.
const rooms = vi.hoisted(() => [] as InstanceType<typeof import("node:events").EventEmitter>[]);
const keyProviders = vi.hoisted(() => [] as { setKey: ReturnType<typeof vi.fn> }[]);
// jsdom has no workers; the fake Room never starts LiveKit's encryption worker anyway.
vi.hoisted(() => {
  globalThis.Worker ??= function Worker() {} as unknown as typeof Worker;
});
vi.mock("livekit-client", async () => {
  const { EventEmitter } = await import("node:events");
  class Participant extends EventEmitter {
    attributes: Record<string, string> = {};
    audioLevel = 0;
    constructor(
      public identity: string,
      public name: string,
      public isAgent: boolean,
    ) {
      super();
    }
    getTrackPublication = () => undefined;
  }
  class Room extends EventEmitter {
    remoteParticipants = new Map<string, Participant>();
    canPlaybackAudio = true;
    localParticipant = {
      isMicrophoneEnabled: false,
      isCameraEnabled: false,
      getTrackPublication: () => undefined,
      setMicrophoneEnabled: vi.fn(async (on: boolean) => {
        this.localParticipant.isMicrophoneEnabled = on;
      }),
      setCameraEnabled: vi.fn(),
      publishData: vi.fn(async () => undefined),
    };
    setE2EEEnabled = vi.fn(async () => undefined);
    connect = vi.fn(async () => undefined);
    disconnect = vi.fn(async () => undefined);
    startAudio = vi.fn(async () => undefined);
    constructor() {
      super();
      rooms.push(this);
    }
    join(identity: string, name: string, isAgent = false) {
      this.remoteParticipants.set(identity, new Participant(identity, name, isAgent));
      this.emit("participantConnected");
    }
  }
  class ExternalE2EEKeyProvider {
    setKey = vi.fn(async () => undefined);
    constructor() {
      keyProviders.push(this);
    }
  }
  return {
    Room,
    ExternalE2EEKeyProvider,
    isE2EESupported: () => true,
    RoomEvent: Object.fromEntries(
      ["ParticipantConnected", "ParticipantDisconnected", "Reconnecting", "Reconnected", "Disconnected", "TrackSubscribed", "TrackUnsubscribed", "TrackMuted", "TrackUnmuted", "LocalTrackPublished", "LocalTrackUnpublished", "AudioPlaybackStatusChanged", "DataReceived"].map((e) => [
        e,
        e.charAt(0).toLowerCase() + e.slice(1),
      ]),
    ),
    ParticipantEvent: { AttributesChanged: "attributesChanged", TrackSubscribed: "trackSubscribed", TrackUnsubscribed: "trackUnsubscribed" },
    Track: { Source: { Camera: "camera", Microphone: "microphone" }, Kind: { Video: "video", Audio: "audio" } },
    createAudioAnalyser: vi.fn(),
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
  localStorage.clear();
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
  expect(nav).toHaveTextContent("TodayUsListsHome");
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
  renderApp("/us?show=questions");
  expect(await screen.findByText("Jo has answered")).toBeInTheDocument();
  expect(screen.queryByText("Lisbon")).not.toBeInTheDocument();
  await userEvent.type(screen.getByLabelText("Your answer"), "Porto");
  await userEvent.click(screen.getByRole("button", { name: "Answer" }));
  expect(await screen.findByText("Lisbon")).toBeInTheDocument();
  expect(screen.getByText("Porto")).toBeInTheDocument();
  expect(await calls.find((c) => c.method === "PUT")?.json()).toEqual({ text: "Porto" });
});

test("suggest asks for fresh questions and your own can be reworded or deleted", async () => {
  const q = { id: "44444444-4444-4444-8444-444444444445", text: "Bset trip?", askedBy: marius.id, createdAt: "2026-10-27T18:00:00Z", revealed: false, answers: [] };
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/questions`, () => json([q])),
    at("GET", `/v1/households/${home.id}/questions/suggestions`, () => json({ questions: ["Which smell means home?", "What made you laugh?"] })),
    at("PUT", `/v1/questions/${q.id}`, () => json({ ...q, text: "Best trip?" })),
    at("DELETE", `/v1/questions/${q.id}`, () => new Response(null, { status: 204 })),
  );
  renderApp("/us?show=questions");
  await userEvent.click(await screen.findByRole("button", { name: "Suggest one" }));
  expect(await screen.findByDisplayValue("Which smell means home?")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Suggest one" }));
  expect(await screen.findByDisplayValue("What made you laugh?")).toBeInTheDocument();
  expect(calls.filter((c) => c.url.endsWith("/questions/suggestions"))).toHaveLength(1);

  await userEvent.click(screen.getByRole("button", { name: "Edit question" }));
  const field = screen.getByLabelText("Your question");
  await userEvent.clear(field);
  await userEvent.type(field, "Best trip?");
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(await screen.findByText("Best trip?")).toBeInTheDocument();
  expect(await calls.find((c) => c.method === "PUT")?.json()).toEqual({ text: "Best trip?" });

  await userEvent.click(screen.getByRole("button", { name: "Edit question" }));
  await userEvent.click(screen.getByRole("button", { name: "Delete" }));
  await userEvent.click(screen.getByRole("button", { name: "Really delete?" }));
  await waitFor(() => expect(calls.some((c) => c.method === "DELETE")).toBe(true));
});

test("watch ideas come from the assistant, with or without a library", async () => {
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/watch`, () => json({ connected: false, libraries: [] })),
    at("POST", `/v1/households/${home.id}/watch/suggestions`, () =>
      json({ ideas: [{ name: "Paddington 2", year: 2017, kind: "movie", reason: "Pure joy." }] }),
    ),
  );
  renderApp("/watch");
  await userEvent.type(await screen.findByRole("textbox", { name: "Ideas for tonight" }), "something light");
  await userEvent.click(screen.getByRole("button", { name: "Get ideas" }));
  expect(await screen.findByText("Paddington 2")).toBeInTheDocument();
  expect(screen.getByText("Pure joy.")).toBeInTheDocument();
  expect(screen.getByText(/Not in your library/)).toBeInTheDocument();
  expect(await calls.find((c) => c.url.endsWith("/watch/suggestions"))?.json()).toEqual({ mood: "something light" });
});

test("watch filters a library, opens a title and leaves out the libraries you turn off", async () => {
  const movies = { id: "a.1", name: "Movies", kind: "movie", server: "Home", owned: true, included: true };
  const kids = { id: "a.2", name: "Kids", kind: "movie", server: "Home", owned: true, included: false };
  const shared = { id: "b.1", name: "Films", kind: "movie", server: "Jo's", owned: false, included: true };
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/watch`, () => json({ connected: true, libraries: [movies, kids, shared] })),
    at("GET", `/v1/households/${home.id}/watch/libraries/a.1/titles`, () =>
      json([
        { id: "a.10", kind: "movie", name: "Amélie", genres: ["Comedy", "Romance"], minutes: 122, watched: false, summary: "Paris, gently." },
        { id: "a.11", kind: "movie", name: "Heat", genres: ["Crime"], minutes: 170, watched: true },
        { id: "a.12", kind: "movie", name: "Paddington 2", genres: ["Comedy"], minutes: 103, watched: false, url: "https://app.plex.tv/p2" },
      ]),
    ),
    at("PUT", `/v1/households/${home.id}/watch/libraries/b.1`, () => new Response(null, { status: 204 })),
    at("GET", `/v1/households/${home.id}/watch/tonight`, () => json({})),
    at("PUT", `/v1/households/${home.id}/watch/tonight`, () =>
      json({ titleId: "a.12", kind: "movie", name: "Paddington 2", url: "https://app.plex.tv/p2", pickedBy: marius.id, pickedAt: "2026-10-30T19:00:00Z" }),
    ),
  );
  renderApp("/watch");
  expect(await screen.findByText("Amélie")).toBeInTheDocument();
  expect(screen.queryByRole("tab", { name: /Kids/ })).not.toBeInTheDocument();

  await userEvent.type(screen.getByRole("searchbox", { name: "Search" }), "amelie");
  expect(screen.queryByText("Paddington 2")).not.toBeInTheDocument();
  await userEvent.clear(screen.getByRole("searchbox", { name: "Search" }));

  await userEvent.click(screen.getByRole("button", { name: "Filter" }));
  const filters = screen.getByRole("dialog", { name: "Filter" });
  await userEvent.click(within(filters).getByRole("switch", { name: "Not seen yet" }));
  await userEvent.click(within(filters).getByRole("radio", { name: "Up to 2 h" }));
  await userEvent.click(within(filters).getByRole("button", { name: "Done" }));
  expect(screen.getByText("1 title")).toBeInTheDocument();
  expect(screen.queryByText("Heat")).not.toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: /Paddington 2/ }));
  const sheet = screen.getByRole("dialog", { name: "Paddington 2" });
  expect(within(sheet).getByRole("link", { name: "Open in Plex" })).toHaveAttribute("href", "https://app.plex.tv/p2");
  await userEvent.click(within(sheet).getByRole("button", { name: "Watch tonight" }));
  const tonight = await screen.findByRole("region", { name: "Tonight" });
  expect(within(tonight).getByText("You picked this")).toBeInTheDocument();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(await calls.find((c) => c.method === "PUT" && c.url.endsWith("/watch/tonight"))?.json()).toEqual({ titleId: "a.12" });

  await userEvent.click(screen.getByRole("button", { name: "Choose" }));
  const choose = screen.getByRole("dialog", { name: "Libraries" });
  expect(within(choose).getByRole("switch", { name: "Kids" })).toHaveAttribute("aria-checked", "false");
  await userEvent.click(within(choose).getByRole("switch", { name: "Films · Jo's" }));
  await waitFor(() => expect(calls.some((c) => c.method === "PUT" && c.url.endsWith("/b.1"))).toBe(true));
  expect(await calls.find((c) => c.method === "PUT" && c.url.endsWith("/b.1"))?.json()).toEqual({ included: false });
  expect(screen.queryByRole("tab", { name: /Films/ })).not.toBeInTheDocument();
});

// A local date as the API writes it, days from today.
function isoDay(offset = 0) {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

test("the timeline shows the month, brings back a year ago and keeps today's line", async () => {
  const month = isoDay().slice(0, 7);
  const moment = (id: string, fields: object) => ({
    id, authorId: jo.id, visibility: "shared", sealed: false, createdAt: "2026-10-01T08:00:00Z", updatedAt: "2026-10-01T08:00:00Z", ...fields,
  });
  const lake = moment("66666666-6666-4666-8666-666666666661", { kind: "moment", day: isoDay(), text: "Lake day", photoUrl: "/v1/moments/x/photo?v=1" });
  const line = moment("66666666-6666-4666-8666-666666666662", { kind: "gratitude", day: isoDay(), text: "Your soup" });
  const mine = moment("66666666-6666-4666-8666-666666666663", { kind: "gratitude", day: isoDay(), text: "The walk", authorId: marius.id });
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/moments`, () => json({ month, moments: [lake, line], earlier: "2025-10" })),
    at("GET", `/v1/households/${home.id}/moments/resurfaced`, () =>
      json({ onThisDay: [moment("66666666-6666-4666-8666-666666666664", { kind: "moment", day: "2025-10-28", text: "First apartment" })] }),
    ),
    at("POST", `/v1/households/${home.id}/moments`, () => json(mine, 201)),
  );
  renderApp("/us");
  expect(await screen.findByText("Lake day")).toBeInTheDocument();
  expect(screen.getByText("1 moment · 1 thank-you")).toBeInTheDocument();
  expect(screen.getByText("On this day · 2025")).toBeInTheDocument();
  // In the card for today and in the month.
  expect(screen.getAllByText("Your soup")).toHaveLength(2);
  expect(screen.getByRole("button", { name: "Earlier" })).toBeInTheDocument();

  await userEvent.type(screen.getByLabelText("What are you grateful for today?"), " The walk ");
  await userEvent.click(screen.getByRole("button", { name: "Just for me" }));
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  const post = calls.find((c) => c.method === "POST");
  expect(await post?.json()).toEqual({ kind: "gratitude", text: "The walk", visibility: "private" });
});

test("a moment's video plays by itself, muted, with sound a tap away, until autoplay is turned off", async () => {
  const video = {
    id: "66666666-6666-4666-8666-666666666665", kind: "moment", authorId: jo.id, day: isoDay(), visibility: "shared", sealed: false, text: "Waves",
    videoUrl: "/v1/moments/y/video?v=1", createdAt: "2026-10-01T08:00:00Z", updatedAt: "2026-10-01T08:00:00Z",
  };
  serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/moments`, () => json({ month: isoDay().slice(0, 7), moments: [video] })),
    at("GET", `/v1/households/${home.id}/moments/resurfaced`, () => json({ onThisDay: [] })),
  );
  const { unmount } = renderApp("/us");
  expect(await screen.findByText("Waves")).toBeInTheDocument();
  const player = document.querySelector("video");
  expect(player?.getAttribute("src")).toBe("/v1/moments/y/video?v=1#t=0.001");
  expect(player?.muted).toBe(true);
  expect(player?.loop).toBe(true);
  expect(player?.controls).toBe(false);
  await userEvent.click(screen.getByRole("button", { name: "Sound on" }));
  expect(player?.muted).toBe(false);
  expect(screen.getByRole("button", { name: "Sound off" })).toHaveAttribute("aria-pressed", "true");
  unmount();

  renderApp("/settings");
  const autoplay = await screen.findByRole("switch", { name: /Play videos automatically/ });
  expect(autoplay).toHaveAttribute("aria-checked", "true");
  await userEvent.click(autoplay);
  expect(autoplay).toHaveAttribute("aria-checked", "false");
  expect(localStorage.getItem("homeloom.autoplayVideos")).toBe("0");
  await userEvent.click(autoplay);
  expect(localStorage.getItem("homeloom.autoplayVideos")).toBeNull();
});

test("keeping a moment sends what happened and the day", async () => {
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/moments`, () => json({ month: isoDay().slice(0, 7), moments: [] })),
    at("GET", `/v1/households/${home.id}/moments/resurfaced`, () => json({ onThisDay: [] })),
    at("POST", `/v1/households/${home.id}/moments`, () =>
      json({ id: "77777777-7777-4777-8777-777777777777", kind: "moment", authorId: marius.id, day: isoDay(-1), visibility: "shared", sealed: false, text: "Pizza night", createdAt: "2026-10-28T08:00:00Z", updatedAt: "2026-10-28T08:00:00Z" }, 201),
    ),
  );
  renderApp("/us");
  expect(await screen.findByText("Your story starts here")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Keep a moment" }));
  const sheet = screen.getByRole("dialog", { name: "New moment" });
  await userEvent.type(within(sheet).getByLabelText("What happened"), "Pizza night");
  fireEvent.change(within(sheet).getByLabelText("Day"), { target: { value: isoDay(-1) } });
  await userEvent.click(within(sheet).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  const post = calls.find((c) => c.method === "POST");
  expect(await post?.json()).toEqual({ kind: "moment", text: "Pizza night", visibility: "shared", day: isoDay(-1) });
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
  Object.defineProperty(navigator, "serviceWorker", { configurable: true, value: Object.assign(new EventTarget(), { ready: Promise.resolve({ pushManager }) }) });
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
  expect(screen.queryByRole("switch", { name: /Notifications/ })).not.toBeInTheDocument();
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
  const curry = { slug: "green-curry", name: "Green curry", tags: ["Thai"], totalTime: "30 min", imageUrl: `/v1/households/${home.id}/recipes/green-curry/image?v=1` };
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

  expect(document.querySelector(".recipe-thumb")).toHaveAttribute("src", curry.imageUrl);
  await userEvent.click(await screen.findByText("Warm and a little spicy."));
  await userEvent.click(await screen.findByLabelText("Rice"));
  expect(document.querySelector(".recipe-hero")).toHaveAttribute("src", curry.imageUrl);
  await userEvent.click(screen.getByRole("button", { name: "Put 2 missing items on the list" }));
  expect(await screen.findByRole("status")).toHaveTextContent("2 items are on “Shopping list” now.");
  expect(await calls.find((c) => c.url.endsWith("/shopping-list"))?.json()).toEqual({ recipe: "green-curry", ingredientIds: ["i1", "i3"] });
});

test("the week plan takes ideas, and its shopping goes to the list in one go", async () => {
  const [today = "", tomorrow = "", third = ""] = nextDays(3);
  const curry = { slug: "green-curry", name: "Green curry", tags: [], imageUrl: `/v1/households/${home.id}/recipes/green-curry/image?v=1` };
  const soup = { slug: "soup", name: "Soup", tags: [] };
  const planned: Record<string, unknown> = { [today]: { day: today, recipe: curry, plannedBy: marius.id } };
  const calls = serve(
    ...signedIn(),
    (req) => {
      const url = new URL(req.url);
      if (req.method === "GET" && url.pathname === `/v1/households/${home.id}/meal-plan`) return json(Object.values(planned));
      if (req.method === "PUT" && url.pathname.startsWith(`/v1/households/${home.id}/meal-plan/`)) {
        const day = url.pathname.split("/").pop() ?? "";
        planned[day] = { day, recipe: soup, plannedBy: marius.id };
        return json(planned[day]);
      }
      return undefined;
    },
    at("POST", `/v1/households/${home.id}/meal-plan/suggestions`, () =>
      json({ chef: true, meals: [{ day: tomorrow, recipe: soup, reason: "Uses the leeks." }, { day: third, recipe: curry }] }),
    ),
    at("GET", `/v1/households/${home.id}/recipes/green-curry`, () => json({ ...curry, steps: [], ingredients: [{ id: "i1", text: "Coconut milk" }, { id: "i2", text: "Rice" }] })),
    at("GET", `/v1/households/${home.id}/recipes/soup`, () => json({ ...soup, steps: [], ingredients: [{ id: "i1", text: "Leeks" }] })),
    at("POST", `/v1/households/${home.id}/meal-plan/shopping-list`, () => json({ added: 2, list: "Shopping list" })),
  );
  renderApp("/cook/week");
  expect(await screen.findByText("Green curry")).toBeInTheDocument();
  await userEvent.type(screen.getByLabelText("Use up"), "leeks");
  await userEvent.click(screen.getByRole("button", { name: "Ideas for 6 open days" }));
  expect(await calls.find((c) => c.url.endsWith("/meal-plan/suggestions"))?.json()).toEqual({ from: today, days: 7, wish: "leeks" });
  expect(await screen.findByText("Uses the leeks.")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Skip Green curry" }));
  await userEvent.click(screen.getByRole("button", { name: "Plan Soup" }));
  expect(await calls.find((c) => c.method === "PUT")?.json()).toEqual({ recipe: "soup" });
  expect(calls.find((c) => c.method === "PUT")?.url).toContain(`/meal-plan/${tomorrow}`);

  await userEvent.click(screen.getByRole("link", { name: "Shopping for the week" }));
  await userEvent.click(await screen.findByLabelText("Rice"));
  await userEvent.click(screen.getByRole("button", { name: "Put 2 missing items on the list" }));
  expect(await screen.findByRole("status")).toHaveTextContent("2 items are on “Shopping list” now.");
  expect(await calls.find((c) => c.url.endsWith("/meal-plan/shopping-list"))?.json()).toEqual({
    recipes: [{ recipe: "green-curry", ingredientIds: ["i1"] }, { recipe: "soup", ingredientIds: ["i1"] }],
  });
});

test("today shows tonight's planned dinner", async () => {
  const [today = ""] = nextDays(1);
  serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/meal-plan`, () => json([{ day: today, recipe: { slug: "soup", name: "Leek soup", tags: [], totalTime: "30 min" }, plannedBy: jo.id }])),
    at("GET", `/v1/households/${home.id}/recipes/soup`, () => json({ slug: "soup", name: "Leek soup", tags: [], steps: [], ingredients: [] })),
  );
  renderApp();
  await userEvent.click(await screen.findByRole("button", { name: /Leek soup/ }));
  expect(await screen.findByRole("heading", { level: 1, name: "Leek soup" })).toBeInTheDocument();
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

test("home turns all lights off at once", async () => {
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/home`, () =>
      json({
        connected: true,
        rooms: [{ id: "living", name: "Living room" }],
        scenes: [],
        lights: [
          { id: "light.sofa", name: "Sofa", room: "living", on: true },
          { id: "light.desk", name: "Desk", room: "living", on: true },
          { id: "light.hall", name: "Hall", on: false },
        ],
        readings: [],
      }),
    ),
    at("POST", `/v1/households/${home.id}/home/lights/light.sofa/toggle`, () => json({ id: "light.sofa", name: "Sofa", on: false })),
    at("POST", `/v1/households/${home.id}/home/lights/light.desk/toggle`, () => json({ id: "light.desk", name: "Desk", on: false })),
  );
  renderApp("/home");
  expect(await screen.findByText("2 lights on")).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "Living room" })).toHaveTextContent("2 on");
  await userEvent.click(screen.getByRole("button", { name: "All off" }));
  expect(await screen.findByText("All lights off")).toBeInTheDocument();
  expect(screen.getByRole("switch", { name: "Sofa" })).toHaveAttribute("aria-checked", "false");
  expect(calls.filter((c) => c.method === "POST").map((c) => new URL(c.url).pathname.split("/").at(-2))).toEqual(["light.sofa", "light.desk"]);
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

test("the app opens at once from what it kept, and offline too", async () => {
  const list = `/v1/households/${home.id}/shopping-list`;
  const milk = { id: "44444444-4444-4444-8444-444444444444", text: "Milk", aisle: "dairy", done: false, createdAt: "2026-10-28T08:00:00Z" };
  serve(...signedIn(), at("GET", list, () => json([milk])));
  const first = renderApp("/lists?show=shopping");
  expect(await screen.findByText("Milk")).toBeInTheDocument();
  first.unmount();

  // No network: the session and the list are there on the first render, without loading.
  vi.mocked(fetch).mockRejectedValue(new TypeError("Load failed"));
  renderApp("/lists?show=shopping");
  expect(screen.getByRole("heading", { level: 1, name: "Lists" })).toBeInTheDocument();
  expect(screen.getByText("Milk")).toBeInTheDocument();
  await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled());
  expect(screen.getByText("Milk")).toBeInTheDocument();
});

test("the shopping list takes items offline and sends them once back online", async () => {
  const list = `/v1/households/${home.id}/shopping-list`;
  const calls = serve(...signedIn(), at("GET", list, () => json([])));
  const first = renderApp("/lists?show=shopping");
  expect(await screen.findByText("Nothing to buy")).toBeInTheDocument();
  first.unmount();

  let online = false;
  vi.spyOn(navigator, "onLine", "get").mockImplementation(() => online);
  vi.mocked(fetch).mockRejectedValue(new TypeError("Load failed"));
  renderApp("/lists?show=shopping");
  await userEvent.type(screen.getByLabelText("Add an item"), "Bread");
  await userEvent.click(screen.getByRole("button", { name: "Add" }));
  expect(await screen.findByText("Bread")).toBeInTheDocument();
  expect(screen.getByRole("status")).toHaveTextContent("You're offline. Your change goes out once you're back.");

  const bread = { id: "44444444-4444-4444-8444-444444444444", text: "Bread", aisle: "bakery", done: false, createdAt: "2026-10-30T08:00:00Z" };
  vi.mocked(fetch).mockImplementation(async (req) => {
    const r = req as Request;
    calls.push(r);
    if (r.method === "POST") return json(bread, 201);
    if (new URL(r.url).pathname === list) return json([bread]);
    return json({}, 404);
  });
  online = true;
  act(() => void window.dispatchEvent(new Event("online")));
  await waitFor(() => expect(calls.filter((c) => c.method === "POST")).toHaveLength(1));
  expect(await calls.find((c) => c.method === "POST")?.json()).toEqual({ text: "Bread" });
  expect(await screen.findByRole("heading", { name: "Bakery" })).toBeInTheDocument();
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  vi.restoreAllMocks();
});

test("signing out elsewhere drops what the app kept", async () => {
  serve(...signedIn());
  const first = renderApp();
  expect(await screen.findByRole("heading", { level: 1, name: "Today" })).toBeInTheDocument();
  first.unmount();

  serve(at("GET", "/v1/me", () => json({ code: "unauthenticated", message: "" }, 401)));
  const second = renderApp();
  expect(await screen.findByRole("button", { name: "Sign in" })).toBeInTheDocument();
  second.unmount();

  serve();
  vi.mocked(fetch).mockRejectedValue(new TypeError("Load failed"));
  renderApp();
  expect(await screen.findByRole("button", { name: "Try again" })).toBeInTheDocument();
});

test("the shopping list adds, ticks and clears items", async () => {
  const list = `/v1/households/${home.id}/shopping-list`;
  const milk = { id: "44444444-4444-4444-8444-444444444444", text: "Milk", aisle: "dairy", done: false, createdAt: "2026-10-28T08:00:00Z" };
  const lime = { id: "55555555-5555-4555-8555-555555555555", text: "Lime", aisle: "produce", recipe: "Green curry", done: true, createdAt: "2026-10-28T08:00:00Z" };
  const calls = serve(
    ...signedIn(),
    at("GET", list, () => json([milk, lime])),
    at("GET", `${list}/suggestions`, () => json([{ text: "Eggs", uses: 3 }, { text: "milk", uses: 2 }])),
    at("POST", `${list}/items`, () => json({ ...milk, text: "Bread" }, 201)),
    at("PUT", `${list}/items/${milk.id}/done`, () => json({ ...milk, done: true })),
    at("DELETE", `${list}/done`, () => new Response(null, { status: 204 })),
    at("PATCH", `${list}/items/${milk.id}`, () => json({ ...milk, aisle: "drinks" })),
  );
  renderApp("/lists?show=shopping");
  expect(await screen.findByText("Milk")).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Dairy & eggs" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: /^Milk/ }));
  await userEvent.click(screen.getByRole("button", { name: "Drinks" }));
  await waitFor(() => expect(calls.find((c) => c.method === "PATCH")?.json()).resolves.toEqual({ aisle: "drinks" }));
  expect(screen.queryByRole("button", { name: "Add" })).toBeInTheDocument();
  expect(calls.some((c) => c.url.includes("/entries"))).toBe(false);

  // Done items are hidden until asked for, and the choice is remembered.
  expect(screen.queryByText("Green curry")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Show 1" }));
  expect(screen.getByText("Green curry")).toBeInTheDocument();
  expect(localStorage.getItem(`homeloom.showDone.${marius.id}.shopping`)).toBe("1");

  // What was bought before is suggested, except what's on the list already.
  const chips = await screen.findByRole("group", { name: "Used before" });
  expect(within(chips).getAllByRole("button").map((b) => b.textContent)).toEqual(["Eggs"]);
  await userEvent.click(within(chips).getByRole("button", { name: "Eggs" }));
  await waitFor(() => expect(calls.some((c) => c.method === "POST")).toBe(true));
  expect(await calls.find((c) => c.method === "POST")?.json()).toEqual({ text: "Eggs" });

  await userEvent.type(screen.getByLabelText("Add an item"), "Bread");
  await userEvent.click(screen.getByRole("button", { name: "Add" }));
  await waitFor(() => expect(calls.filter((c) => c.method === "POST")).toHaveLength(2));
  expect(await calls.filter((c) => c.method === "POST")[1]?.json()).toEqual({ text: "Bread" });
  await waitFor(() => expect(screen.getByLabelText("Add an item")).toHaveValue(""));

  await userEvent.click(screen.getByRole("checkbox", { name: "Milk" }));
  await waitFor(() => expect(calls.some((c) => c.method === "PUT")).toBe(true));
  await userEvent.click(screen.getByRole("button", { name: "Clear" }));
  await waitFor(() => expect(calls.some((c) => c.method === "DELETE" && c.url.endsWith("/done"))).toBe(true));
});

test("done reminders stay hidden until shown", async () => {
  serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([entry({ text: "Bins out" }), entry({ id: "66666666-6666-4666-8666-666666666666", text: "Pay rent", doneAt: "2026-10-28T08:00:00Z" })])),
  );
  renderApp("/lists");
  expect(await screen.findByText("Bins out")).toBeInTheDocument();
  expect(screen.queryByText("Pay rent")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Show 1" }));
  expect(screen.getByText("Pay rent")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Hide" }));
  expect(screen.queryByText("Pay rent")).not.toBeInTheDocument();
});

test("the calendar never shows reminders while its events load", async () => {
  serve(
    ...signedIn(),
    // Events never arrive, so whatever the calendar shows is what it shows while loading.
    (req) => (new URL(req.url).searchParams.get("kind") === "reminder" ? at("GET", `/v1/households/${home.id}/entries`, () => json([entry({ text: "Bins out" })]))(req) : undefined),
  );
  renderApp("/lists");
  expect(await screen.findByText("Bins out")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("tab", { name: "Calendar" }));
  expect(screen.queryByText("Bins out")).not.toBeInTheDocument();
});

test("the quick-add sheet suggests reminders set before", async () => {
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([])),
    at("GET", `/v1/households/${home.id}/entries/suggestions`, () => json([{ text: "Water the plants", uses: 4 }, { text: "Call mum", uses: 1 }])),
    at("POST", `/v1/households/${home.id}/entries`, () => json(entry({ text: "Water the plants" }), 201)),
  );
  renderApp("/lists");
  await screen.findByText("Nothing to remember");
  await userEvent.click(screen.getByRole("button", { name: "New reminder" }));
  const sheet = screen.getByRole("dialog", { name: "New reminder" });
  await userEvent.type(within(sheet).getByLabelText("Remind about"), "wat");
  const chips = await within(sheet).findByRole("group", { name: "Used before" });
  expect(within(chips).getAllByRole("button").map((b) => b.textContent)).toEqual(["Water the plants"]);
  await userEvent.click(within(chips).getByRole("button", { name: "Water the plants" }));
  expect(within(sheet).getByLabelText("Remind about")).toHaveValue("Water the plants");
  await userEvent.click(within(sheet).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(calls.some((c) => c.method === "POST")).toBe(true));
  expect(await calls.find((c) => c.method === "POST")?.json()).toMatchObject({ text: "Water the plants" });
});

test("the quick-add sheet sets a shared reminder for both", async () => {
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([])),
    at("POST", `/v1/households/${home.id}/entries`, () => json(entry({ text: "Call the landlord" }), 201)),
  );
  renderApp("/lists");
  expect(await screen.findByText("Nothing to remember")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "New reminder" }));
  const sheet = screen.getByRole("dialog", { name: "New reminder" });
  expect(within(sheet).queryByRole("radiogroup", { name: "What to add" })).not.toBeInTheDocument();
  await userEvent.type(within(sheet).getByLabelText("Remind about"), "Call the landlord");
  expect(within(within(sheet).getByRole("radiogroup", { name: "Who can see this" })).getByRole("radio", { name: /Both of us/ })).toBeChecked();
  expect(within(sheet).getByText("Jo sees it right away.")).toBeInTheDocument();
  await userEvent.click(within(sheet).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(await calls.find((c) => c.method === "POST")?.json()).toEqual({ kind: "reminder", text: "Call the landlord", visibility: "shared", repeat: "never" });
});

test("the quick-add sheet picks time, repeat and who with taps, not dropdowns", async () => {
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([])),
    at("POST", `/v1/households/${home.id}/entries`, () => json(entry({ text: "Bins" }), 201)),
  );
  renderApp("/lists");
  expect(await screen.findByText("Nothing to remember")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "New reminder" }));
  const sheet = screen.getByRole("dialog", { name: "New reminder" });
  expect(within(sheet).queryByRole("combobox")).not.toBeInTheDocument();
  await userEvent.type(within(sheet).getByLabelText("Remind about"), "Bins");
  expect(within(sheet).queryByRole("radiogroup", { name: "Repeat" })).not.toBeInTheDocument();
  fireEvent.change(within(sheet).getByLabelText("When"), { target: { value: "2030-05-06T19:30" } });
  await userEvent.click(within(within(sheet).getByRole("radiogroup", { name: "Repeat" })).getByRole("radio", { name: "Every week" }));
  await userEvent.click(within(within(sheet).getByRole("radiogroup", { name: "For" })).getByRole("radio", { name: "Me" }));
  await userEvent.click(within(sheet).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  const body = await calls.find((c) => c.method === "POST")?.json();
  expect(body).toMatchObject({ text: "Bins", repeat: "weekly", assigneeId: expect.any(String), dueAt: new Date("2030-05-06T19:30").toISOString() });
});

test("a sheet closes on Escape", async () => {
  serve(...signedIn(), at("GET", `/v1/households/${home.id}/entries`, () => json([])));
  renderApp("/lists");
  expect(await screen.findByText("Nothing to remember")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "New reminder" }));
  expect(screen.getByRole("dialog", { name: "New reminder" })).toBeInTheDocument();
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

test("add follows the list you're in, and only Today asks what to add", async () => {
  serve(...signedIn(), at("GET", `/v1/households/${home.id}/entries`, () => json([])));
  renderApp("/lists?show=note");
  // A note opens on its own page, with no choice of what to add.
  await userEvent.click(await screen.findByRole("button", { name: "New note" }));
  expect(await screen.findByLabelText("Title")).toBeInTheDocument();
  expect(screen.queryByRole("radiogroup", { name: "What to add" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Notes" }));

  await userEvent.click(screen.getByRole("link", { name: "Today" }));
  await userEvent.click(await screen.findByRole("button", { name: "Add" }));
  const sheet = screen.getByRole("dialog", { name: "New reminder" });
  await userEvent.click(within(sheet).getByRole("radio", { name: "Note" }));
  expect(screen.getByRole("dialog", { name: "New note" })).toBeInTheDocument();
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

test("a note opens as its own page to read, and saves its title as a heading on the way back", async () => {
  const note = entry({ kind: "note", text: "# Trip\n\nPack **light**", title: "Trip" });
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([note])),
    at("GET", `/v1/entries/${note.id}`, () => json(note)),
    at("PUT", `/v1/entries/${note.id}`, () => json(note)),
  );
  renderApp("/lists?show=note");
  expect(await screen.findByText("Pack light")).toBeInTheDocument();
  await userEvent.click(screen.getByText("Trip"));
  expect(await screen.findByRole("heading", { name: "Trip" })).toBeInTheDocument();
  expect(screen.getByText("light").tagName).toBe("STRONG");
  await userEvent.click(screen.getByRole("radio", { name: "Write" }));
  const title = screen.getByLabelText("Title");
  await userEvent.clear(title);
  await userEvent.type(title, "Rome trip");
  await userEvent.click(screen.getByRole("button", { name: "Notes" }));
  expect(await screen.findByRole("tab", { name: "Notes" })).toHaveAttribute("aria-selected", "true");
  expect(await calls.find((c) => c.method === "PUT")?.json()).toEqual({ text: "# Rome trip\n\nPack **light**", visibility: "shared", repeat: "never" });
});

test("discarding a note puts back what it said when opened", async () => {
  const note = entry({ kind: "note", text: "# Trip\n\nPack light", title: "Trip" });
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([note])),
    at("GET", `/v1/entries/${note.id}`, () => json(note)),
    at("PUT", `/v1/entries/${note.id}`, () => json(note)),
  );
  vi.spyOn(window, "confirm").mockReturnValue(true);
  renderApp(`/lists/notes/${note.id}`);
  await userEvent.click(await screen.findByRole("radio", { name: "Write" }));
  expect(screen.queryByRole("button", { name: "Discard" })).not.toBeInTheDocument();
  await userEvent.type(screen.getByLabelText("Title"), " to Rome");
  await userEvent.click(screen.getByRole("button", { name: "Discard" }));
  expect(await screen.findByRole("tab", { name: "Notes" })).toHaveAttribute("aria-selected", "true");
  // Nothing had been saved yet, so nothing is written back.
  expect(calls.some((c) => c.method === "PUT")).toBe(false);
});

test("a note is deleted from its page", async () => {
  const note = entry({ kind: "note", text: "# Trip", title: "Trip" });
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([])),
    at("GET", `/v1/entries/${note.id}`, () => json(note)),
    at("DELETE", `/v1/entries/${note.id}`, () => new Response(null, { status: 204 })),
  );
  vi.spyOn(window, "confirm").mockReturnValue(true);
  renderApp(`/lists/notes/${note.id}`);
  await userEvent.click(await screen.findByRole("button", { name: "Delete" }));
  await screen.findByText("No notes yet");
  expect(calls.some((c) => c.method === "DELETE")).toBe(true);
});

test("an empty new note is never saved", async () => {
  const calls = serve(...signedIn(), at("GET", `/v1/households/${home.id}/entries`, () => json([])));
  renderApp("/lists/notes/new");
  await userEvent.click(await screen.findByRole("button", { name: "Notes" }));
  await screen.findByText("No notes yet");
  expect(calls.some((c) => c.method === "POST")).toBe(false);
});

test("a new note is made with its first words", async () => {
  const made = entry({ id: "99999999-9999-4999-8999-999999999999", kind: "note", text: "# Groceries" });
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([made])),
    at("POST", `/v1/households/${home.id}/entries`, () => json(made, 201)),
  );
  renderApp("/lists/notes/new");
  await userEvent.type(await screen.findByLabelText("Title"), "Groceries");
  await userEvent.click(screen.getByRole("button", { name: "Notes" }));
  await waitFor(() => expect(calls.some((c) => c.method === "POST")).toBe(true));
  expect(await calls.find((c) => c.method === "POST")?.json()).toEqual({ kind: "note", text: "# Groceries", visibility: "shared", repeat: "never" });
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
  const next = await screen.findByRole("region", { name: "Next up" });
  expect(within(next).getByText("Water the plants")).toBeInTheDocument();
  expect(within(next).getByText("Dinner with Jo")).toBeInTheDocument();
  expect(screen.queryByText("Next year")).not.toBeInTheDocument();
});

const chore = (over: Record<string, unknown> = {}) => ({
  id: "99999999-9999-4999-8999-999999999999", title: "Bins", points: 2, repeat: "weekly", rotate: true, assigneeId: marius.id, dueOn: "2026-01-01", createdAt: "2026-01-01T08:00:00Z", ...over,
});

test("today's chores are ticked off on Today", async () => {
  let done = false;
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([])),
    at("GET", `/v1/households/${home.id}/chores`, () => json(done ? [chore({ dueOn: "2099-01-01", assigneeId: jo.id })] : [chore(), chore({ id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", title: "Windows", dueOn: "2099-01-01" })])),
    at("POST", `/v1/households/${home.id}/chores/${chore().id}/done`, () => {
      done = true;
      return json({ id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", title: "Bins", points: 2, userId: marius.id, day: "2026-01-01", doneAt: "2026-01-01T08:00:00Z", thankedBy: [] }, 201);
    }),
  );
  renderApp("/");
  const card = await screen.findByRole("region", { name: "Chores today" });
  expect(within(card).queryByText("Windows")).not.toBeInTheDocument();
  await userEvent.click(within(card).getByRole("checkbox", { name: "Bins" }));
  await waitFor(() => expect(screen.queryByRole("region", { name: "Chores today" })).not.toBeInTheDocument());
  expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/done"))).toBe(true);
});

test("the chores page shows who did what and thanks into the gratitude wall", async () => {
  const completion = { id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", choreId: chore().id, title: "Bins", points: 2, userId: jo.id, day: "2026-01-01", doneAt: "2026-01-01T08:00:00Z", thankedBy: [] as string[] };
  const calls = serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/chores`, () => json([chore({ assigneeId: marius.id })])),
    at("GET", `/v1/households/${home.id}/chore-completions`, () => json([completion])),
    at("GET", `/v1/households/${home.id}/chore-fairness`, () =>
      json({
        week: { from: "2025-12-29", members: [{ userId: marius.id, points: 0, count: 0 }, { userId: jo.id, points: 2, count: 1 }] },
        month: { from: "2026-01-01", members: [{ userId: marius.id, points: 0, count: 0 }, { userId: jo.id, points: 2, count: 1 }] },
      }),
    ),
    at("POST", `/v1/households/${home.id}/chore-completions/${completion.id}/thanks`, () => json({ completion: { ...completion, thankedBy: [marius.id] }, gratitude: { id: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", kind: "gratitude", authorId: marius.id, day: "2026-01-01", visibility: "shared", sealed: false, createdAt: "2026-01-01T08:00:00Z", updatedAt: "2026-01-01T08:00:00Z" } })),
  );
  renderApp("/chores");
  const fair = await screen.findByRole("region", { name: "Who did what" });
  expect(within(fair).getByText("2 points")).toBeInTheDocument();
  await userEvent.click(await screen.findByRole("button", { name: "Thank Jo" }));
  expect(await screen.findByText("Jo hears your thanks, and it's on your gratitude wall.")).toBeInTheDocument();
  const post = calls.find((c) => c.method === "POST");
  expect(await post?.json()).toEqual({ gratitude: "Thank you, Jo, for “Bins”." });
});

test("today greets you and brings back a thank-you", async () => {
  serve(
    ...signedIn(),
    at("GET", `/v1/households/${home.id}/entries`, () => json([])),
    at("GET", `/v1/households/${home.id}/moments/resurfaced`, () =>
      json({ onThisDay: [], gratitude: { id: "88888888-8888-4888-8888-888888888888", kind: "gratitude", authorId: jo.id, day: "2026-01-02", visibility: "shared", sealed: false, text: "Pancakes", createdAt: "2026-01-02T08:00:00Z", updatedAt: "2026-01-02T08:00:00Z" } }),
    ),
  );
  renderApp("/");
  expect(await screen.findByRole("heading", { level: 2, name: /Marius/ })).toBeInTheDocument();
  expect(screen.getByText("You and Jo")).toBeInTheDocument();
  expect(await screen.findByText("Nothing planned today. Time for each other.")).toBeInTheDocument();
  expect(await screen.findByRole("button", { name: /Jo was grateful.*Pancakes/ })).toBeInTheDocument();
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
const callJoin = () => json({ id: callId, url: "wss://home.example", token: "t", e2eeKey: "callkey", room: `call-${home.id}-${callId}` });
type FakeRoom = {
  setE2EEEnabled: ReturnType<typeof vi.fn>;
  connect: ReturnType<typeof vi.fn>;
  disconnect: ReturnType<typeof vi.fn>;
  join: (id: string, name: string, isAgent?: boolean) => void;
  remoteParticipants: Map<string, InstanceType<typeof import("node:events").EventEmitter> & { attributes: Record<string, string> }>;
};

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
  // The call is end-to-end encrypted with the key from the API before it connects.
  expect(keyProviders.at(-1)?.setKey).toHaveBeenCalledWith("callkey");
  expect(room.setE2EEEnabled).toHaveBeenCalledWith(true);

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

test("reactions float up for everyone, and only known ones show", async () => {
  rooms.length = 0;
  serve(
    at("GET", "/v1/me", () => json(marius)),
    at("GET", "/v1/households", () => json([home])),
    at("GET", `/v1/households/${home.id}`, household),
    at("GET", `/v1/households/${home.id}/questions`, () => json([])),
    at("POST", `/v1/households/${home.id}/calls`, callJoin),
  );
  renderApp("/us");
  await userEvent.click(await screen.findByRole("button", { name: "Call Jo" }));
  await screen.findByText("Calling…");
  expect(screen.queryByRole("button", { name: "React" })).not.toBeInTheDocument();
  const room = rooms[0] as unknown as FakeRoom & { emit: (e: string, ...a: unknown[]) => void; localParticipant: { publishData: ReturnType<typeof vi.fn> } };
  act(() => room.join(jo.id, "Jo"));

  await userEvent.click(await screen.findByRole("button", { name: "React" }));
  await userEvent.click(within(screen.getByRole("group", { name: "React" })).getByRole("button", { name: "🎉" }));
  expect(room.localParticipant.publishData).toHaveBeenCalledWith(new TextEncoder().encode("🎉"), { reliable: true, topic: "reaction" });
  expect(screen.getByText("🎉", { selector: ".call-reaction" })).toBeInTheDocument();

  act(() => room.emit("dataReceived", new TextEncoder().encode("❤️"), undefined, undefined, "reaction"));
  act(() => room.emit("dataReceived", new TextEncoder().encode("hello"), undefined, undefined, "reaction"));
  expect(screen.getByText("❤️", { selector: ".call-reaction" })).toBeInTheDocument();
  expect(screen.queryByText("hello")).not.toBeInTheDocument();

  // Someone taking a photo says so.
  expect(screen.getByRole("button", { name: "Take a photo" })).toBeInTheDocument();
  act(() => room.emit("dataReceived", new Uint8Array(), { identity: jo.id, name: "Jo", isAgent: false }, undefined, "photo"));
  expect(screen.getByText("Jo took a photo")).toBeInTheDocument();
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

test("answering a call that's already over says so", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  rooms.length = 0;
  serve(
    at("GET", "/v1/me", () => json(marius)),
    at("GET", "/v1/households", () => json([home])),
    at("GET", `/v1/households/${home.id}`, household),
    at("GET", `/v1/households/${home.id}/questions`, () => json([])),
    at("POST", `/v1/households/${home.id}/calls/${callId}/join`, callJoin),
  );
  try {
    renderApp(`/call/${callId}?from=${jo.id}`);
    await userEvent.click(await screen.findByRole("button", { name: "Answer" }));
    await waitFor(() => expect((rooms[0] as unknown as FakeRoom | undefined)?.connect).toHaveBeenCalled());
    await act(() => vi.advanceTimersByTimeAsync(5_000));
    expect(await screen.findByText("Call ended")).toBeInTheDocument();
  } finally {
    vi.useRealTimers();
  }
});

test("a tapped notification opens its screen in the open app", async () => {
  const sw = new EventTarget();
  Object.defineProperty(navigator, "serviceWorker", { value: sw, configurable: true });
  serve(
    at("GET", "/v1/me", () => json(marius)),
    at("GET", "/v1/households", () => json([home])),
    at("GET", `/v1/households/${home.id}`, household),
    at("GET", `/v1/households/${home.id}/questions`, () => json([])),
  );
  renderApp("/");
  await screen.findByRole("link", { name: "Lists" });
  act(() => {
    sw.dispatchEvent(new MessageEvent("message", { data: { type: "homeloom:open", path: "https://evil.example/" } }));
    sw.dispatchEvent(new MessageEvent("message", { data: { type: "homeloom:open", path: `/call/${callId}?from=${jo.id}` } }));
  });
  expect(await screen.findByRole("heading", { name: "Jo is calling" })).toBeInTheDocument();
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
    at("POST", `/v1/households/${home.id}/calls/homeloom`, () => json({ id: callId, url: "wss://home.example", token: "t", e2eeKey: "callkey", room: "homeloom-x" }, 201)),
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
  // The orb follows what the agent is doing.
  expect(screen.getByText("Joining…")).toBeInTheDocument();
  const agent = room.remoteParticipants.get("agent-1");
  if (!agent) throw new Error("no agent");
  act(() => {
    agent.attributes = { "lk.agent.state": "thinking" };
    agent.emit("attributesChanged");
  });
  expect(screen.getByText("Thinking…")).toBeInTheDocument();
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

test("settings saves how Homeloom sounds", async () => {
  const calls = serve(
    ...signedIn(),
    at("GET", "/v1/me/voice", () => json({ available: true, settings: { voice: "coral", speed: 1, tone: "warm" } })),
    at("PUT", "/v1/me/voice", () => json({ voice: "coral", speed: 1, tone: "calm" })),
  );
  renderApp("/settings");
  await userEvent.click(await screen.findByRole("tab", { name: "Calm" }));
  expect(screen.getByRole("tab", { name: "Calm" })).toHaveAttribute("aria-selected", "true");
  await waitFor(() => expect(calls.some((c) => c.method === "PUT" && c.url.endsWith("/v1/me/voice"))).toBe(true));
  expect(await calls.find((c) => c.method === "PUT")?.json()).toEqual({ voice: "coral", speed: 1, tone: "calm" });
  expect(screen.getByRole("button", { name: "Listen" })).toBeInTheDocument();
});

test("settings hides the voice when Homeloom can't speak", async () => {
  serve(...signedIn(), at("GET", "/v1/me/voice", () => json({ available: false, settings: { voice: "coral", speed: 1, tone: "warm" } })));
  renderApp("/settings");
  expect(await screen.findByText("Our place", { exact: false })).toBeInTheDocument();
  expect(screen.queryByText("Homeloom's voice")).not.toBeInTheDocument();
});
