import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { App } from "./App";
import i18n from "./i18n";
import { SessionProvider } from "./session";

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
afterEach(() => vi.unstubAllGlobals());

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
    at("POST", `/v1/households/${home.id}/shopping-list`, () => json({ added: 2, list: "Woche" })),
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
  expect(await screen.findByRole("status")).toHaveTextContent("2 items are on “Woche” now.");
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
