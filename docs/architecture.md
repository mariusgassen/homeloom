# Architecture

Homeloom is an API plus separate clients. The API is the only component that holds secrets or talks to Mealie, Home Assistant and the LLM; every client (the PWA, a future iOS app, the MCP server, third-party apps) goes through the same contract and the same permission checks.

```
 PWA ─┐
 iOS ─┼──▶ homeloom-api ──▶ homeloom-postgres
 MCP ─┤    (Go)          ──▶ homeloom-redis
 3rd ─┘                  ──▶ authentik · Mealie · Home Assistant · Claude
```

## Decisions

| Topic | Decision |
|---|---|
| Backend | Go, hexagonal architecture |
| Contract | API first: `apps/api/api/openapi.yaml` is the source of truth |
| HTTP | chi + oapi-codegen (strict server), requests validated against the spec |
| Database | PostgreSQL 18, sqlc for queries, goose migrations embedded in the binary |
| Cache / realtime | Redis 8 (user cache now; pub/sub fan-out, rate limits and job queues later) |
| Identity | authentik; the API is an OAuth2 resource server and only verifies access tokens |
| LLM | Claude by default, behind a provider port so it can be swapped |
| Frontend | Mobile-first PWA in `apps/web` (Vite, React, TypeScript, vite-plugin-pwa), served by Caddy on the same origin as the API; both users are on iOS |
| Deployment | One Docker Compose resource on Coolify; every service is prefixed `homeloom-` |
| Languages | German and English from the start, English as fallback; `Accept-Language` negotiated per request |
| Versioning | SemVer via release-please from Conventional Commit PR titles; images `ghcr.io/mariusgassen/homeloom-api:X.Y.Z` and `homeloom-web:X.Y.Z` per release |

## Hexagonal layout

```
apps/api/
  api/openapi.yaml             the contract
  cmd/homeloom-api/            composition root: reads config, wires adapters into the core
  internal/core/
    domain/                    entities and business errors, no dependencies
    port/                      driving ports (services the core offers) and driven ports (what it needs)
    service/                   use cases; all authorisation lives here
  internal/adapter/
    http/                      driving: serves the contract (oapi/ is generated)
    postgres/                  driven: repositories (sqlcgen/ is generated, migrations/ embedded)
    redis/                     driven: sessions, caches in front of other ports, health
    oidc/                      driven: verifies authentik tokens, runs the browser login
    memory/                    driven: in-memory repositories and a fake login provider for tests
  internal/config/             environment variables
```

Rules:

- `core` imports nothing from `adapter`. Adapters depend on ports, never on each other, except decorators such as `redis.UserCache` that wrap another port.
- Authorisation is decided in `core/service`, so a future MCP adapter or job runner cannot bypass it. Anything a caller may not see is reported as `ErrNotFound`, so ids cannot be probed.
- Handlers only translate between generated API types and domain types.

## API-first workflow

1. Change `api/openapi.yaml`.
2. Run `go generate ./...` in `apps/api` (regenerates `internal/adapter/http/oapi` and `internal/adapter/postgres/sqlcgen`).
3. The compiler now lists every handler that no longer matches the contract; implement them.

CI fails if generated code is out of date. Which endpoints need a token is defined by `security` in the spec; the request validator calls the authenticator for exactly those operations.

## Authentication

- **Browser (PWA):** a confidential authentik client. The browser navigates to `/auth/login`; the API redirects to authentik (authorization code flow with PKCE and nonce), redeems the code at `/auth/callback` with the client secret, and stores the session in Redis. The browser only gets an HttpOnly, Secure, SameSite=Lax `homeloom_session` cookie; tokens and the secret never reach it. `POST /auth/logout` deletes the session and returns authentik's end-session URL.
- **Other clients (iOS, MCP, third-party):** each is its own authentik provider and sends its access token as `Authorization: Bearer`. The API verifies the JWT against the issuer's JWKS and accepts `OIDC_CLIENT_ID` plus any client ID in `OIDC_CLIENT_IDS` (add it to the compose file once such a client exists). The provider must have a signing key so tokens are JWTs.
- The spec offers both schemes on every protected operation; either one is enough.
- On the first request a local `users` row is created from the `sub`; later requests are served from the Redis user cache unless the profile changed.
- If authentik or Redis cannot be reached the API returns `503 auth_unavailable`, not `401`, and health checks keep working. A failed login redirects to `/?auth_error=login_failed` or `/?auth_error=auth_unavailable`.
- Sessions are stored under a hash of their id, so a Redis dump holds no usable cookies. They last `SESSION_TTL` (30 days by default).

## Households and questions

- **Invites:** a member creates a one-time code (`POST /v1/households/{id}/invites`), shared as `PUBLIC_URL/join/<code>` and valid for 7 days. Only its SHA-256 is stored. Accepting it (`POST /v1/invites/{code}`) locks the invite row, so a code joins exactly one person. A used, expired or unknown code is the same `404 invite_invalid`. The PWA keeps a pending code in localStorage while the person signs in.
- **Reveal together:** any member asks a question. Answers stay sealed, and the API leaves others' text out, until every current member (at least two) has answered. The last answer reveals the question in the same transaction, under a lock on the question row, so two simultaneous answers cannot both miss the reveal. You can change your answer until then; afterwards it is fixed (`409 already_revealed`).
- Sealing happens in `core/service`, so later adapters (MCP, notifications) cannot leak an answer early.

## Cooking

- **Mealie** (`adapter/mealie`, port `RecipeBook`) holds the recipes and the shopping list. The API reaches it only through Mealie's MCP server (`MEALIE_MCP_URL`, bearer `MEALIE_MCP_TOKEN`), which exposes one tool per Mealie endpoint and holds Mealie's own credentials; the same server can later be handed to Claude for the assistant. The MCP session is reused and replaced once if the server has forgotten it. Cooking is optional: without `MEALIE_MCP_URL` the cook endpoints answer `503 cooking_unavailable`, and so do they when the MCP server or Mealie is down or refuses a token. Each call is bounded to 8 s. `MEALIE_URL`, if set, is only used for "Open in Mealie" links.
- **Claude** (`adapter/claude`, port `Chef`) picks up to three recipes for a mood from the whole collection (one short line per recipe, cached as a prompt prefix) and writes why, in the request's language. The answer is constrained to a JSON schema, and the service drops any slug that isn't in the collection. Without `ANTHROPIC_API_KEY`, or when Claude is down, refuses or finds nothing, three recipes are picked at random (`chef: false`). One pick is bounded to 30 s.
- Missing ingredients go to `MEALIE_SHOPPING_LIST_ID`, or to the list that changed last, through Mealie's "add recipe to list" so they stay linked to the recipe. The PWA asks what you already have and sends the rest.

## Smart home

- **Home Assistant** (`adapter/homeassistant`, port `SmartHome`) is optional: without `HOME_ASSISTANT_URL` the home endpoints answer `503 home_unavailable`, and so do they when Home Assistant is down. Each call is bounded to 5 s.
- **Each person connects their own account** through Home Assistant's OAuth sign-in, so every switch is made, and logged, as that person. The client id is `PUBLIC_URL/` and the redirect is `PUBLIC_URL/auth/homeassistant/callback`; Home Assistant needs no client registration for that. The state is bound to the person who started the sign-in (Redis, 10 minutes). The refresh token is sealed with AES-GCM under `HOMELOOM_SECRET_KEY` (`adapter/secretbox`, a decorator on the repository) before it reaches Postgres; access tokens live only in the API's memory. When Home Assistant rejects the refresh token, the connection is dropped and the app asks to connect again. Disconnecting revokes the token.
- The whitelist is the port itself: it can list scenes, lights and a few read-only readings (temperature, humidity, power, CO₂; open windows, doors and garage doors, motion, water, smoke), turn a scene on and switch a light. Locks, alarms and covers can't be switched. The contract only accepts `scene.*` and `light.*` ids, the service only switches ids Home Assistant listed, and the adapter checks the domain once more.
- **Rooms** are Home Assistant's areas. The adapter reads the area, device and entity registries over Home Assistant's WebSocket API (the only place they're offered, and readable without admin rights); an entity's room is its own area or its device's. Members of a group helper are left out so the group shows as one control, and so are entities hidden in Home Assistant or marked as config or diagnostic. If the registries can't be read, the home still shows, without rooms.

## PWA

```
apps/web/
  src/api/            typed client; schema.gen.ts is generated from the OpenAPI contract
  src/ui/, styles/    the design system's tokens and components, ported to React
  src/screens/        one file per screen; tabs: Today, Cook, Us, Lists, Home
  src/i18n/           German and English copy
  Caddyfile           static files plus the reverse proxy to the API
```

- The app never sees a token: it starts sign-in with a full navigation to `/auth/login` and then calls the API with the session cookie.
- The service worker precaches the app shell and never handles `/auth`, `/v1` or the health checks.
- Installable on iOS from Safari's "Add to Home Screen". Web Push needs that install on iOS; in Safari itself Settings explains how to add the app.

## Notifications

- **Web Push with VAPID** (`adapter/webpush`, port `Pusher`) is optional: without `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY`, `GET /v1/push` answers `503 push_unavailable` and the PWA hides the switch. `homeloom-api vapid-keys` prints a new pair. `VAPID_SUBJECT` defaults to `PUBLIC_URL`.
- Subscriptions are per person and device (`push_subscriptions`, keyed by endpoint), each with the language it was made in, so every device gets its notification in its own language from the server's catalog. Only the push services of Apple, Google, Mozilla and Microsoft are accepted as endpoints, because the API posts to them. One that the push service reports gone (404/410) is deleted.
- Turning it on is a tap in Settings: the permission prompt comes first (iOS only shows it for a user gesture), then `PushManager.subscribe` with the server's key. The service worker (`public/push-sw.js`, imported into the generated one) shows the notification and opens its page when tapped.
- The core decides who hears what. Notifications are sent in the background, bounded to 10 s per push service and 30 s per round, and never fail the request that caused them; on shutdown the API waits for those under way.
- So far: when the last member answers a question, everyone else hears that the answers are open.

## Failure handling

- `/healthz` reports the process is up; `/readyz` checks Postgres and Redis and returns `503` when one is down.
- Redis outages degrade to direct database access; each Redis call is bounded to 250 ms.
- Mealie, Home Assistant, the LLM and Web Push follow the same rule: their outage disables a feature, never the app.

## Deployment on Coolify

- Create a Docker Compose resource from this repository (`docker-compose.yml`).
- Set `POSTGRES_PASSWORD`, `PUBLIC_URL`, `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID` and `OIDC_CLIENT_SECRET`.
- In authentik, register `PUBLIC_URL/auth/callback` as the redirect URI and `PUBLIC_URL/` as the post-logout redirect.
- Assign the public domain to `homeloom-web` (port 8080). Its Caddy serves the PWA and forwards `/auth/*`, `/v1/*`, `/healthz` and `/readyz` to `homeloom-api`, so app and API share one origin and the session cookie stays first-party. The API, Postgres and Redis stay internal.

## Planned components

| Component | Shape |
|---|---|
| MCP server | Second driving adapter on the same services, served at `/mcp`, with its own authentik client |
| Reminders & notes | Core entries with `private / shared / reveal_after` visibility; jobs via Redis, delivered through the existing Web Push |
| Home Assistant media | Media players join scenes and lights in the whitelist |
| LiveKit calls with a Homeloom agent | Self-hosted LiveKit on Coolify; the API issues room tokens; the agent is a separate service (LiveKit Agents) that calls the API |
| Third-party apps | Per-app authentik clients or tokens, outgoing webhooks, iCal feed for reminders |
