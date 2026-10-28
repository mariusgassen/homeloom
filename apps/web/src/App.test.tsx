import { render, screen, waitFor } from "@testing-library/react";
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
