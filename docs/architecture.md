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
| Frontend | Mobile-first PWA, separate app; both users are on iOS |
| Deployment | One Docker Compose resource on Coolify; every service is prefixed `homeloom-` |
| Languages | German and English from the start, English as fallback; `Accept-Language` negotiated per request |
| Versioning | SemVer via release-please from Conventional Commit PR titles; image `ghcr.io/mariusgassen/homeloom-api:X.Y.Z` per release |

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
- **Other clients (iOS, MCP, third-party):** each is its own authentik provider and sends its access token as `Authorization: Bearer`. The API verifies the JWT against the issuer's JWKS and accepts `OIDC_CLIENT_ID` plus any client ID in `OIDC_CLIENT_IDS`. The provider must have a signing key so tokens are JWTs.
- The spec offers both schemes on every protected operation; either one is enough.
- On the first request a local `users` row is created from the `sub`; later requests are served from the Redis user cache unless the profile changed.
- If authentik or Redis cannot be reached the API returns `503 auth_unavailable`, not `401`, and health checks keep working. A failed login redirects to `/?auth_error=login_failed` or `/?auth_error=auth_unavailable`.
- Sessions are stored under a hash of their id, so a Redis dump holds no usable cookies. They last `SESSION_TTL` (30 days by default).

## Failure handling

- `/healthz` reports the process is up; `/readyz` checks Postgres and Redis and returns `503` when one is down.
- Redis outages degrade to direct database access; each Redis call is bounded to 250 ms.
- Mealie, Home Assistant and the LLM will follow the same rule: their outage disables a feature, never the app.

## Deployment on Coolify

- Create a Docker Compose resource from this repository (`docker-compose.yml`).
- Set `POSTGRES_PASSWORD`, `PUBLIC_URL`, `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID` and `OIDC_CLIENT_SECRET`; `OIDC_CLIENT_IDS` only for further bearer clients.
- In authentik, register `PUBLIC_URL/auth/callback` as the redirect URI and `PUBLIC_URL/` as the post-logout redirect.
- Assign the public domain to `homeloom-api` (port 8080). Once the PWA exists it shares the domain: `/auth/*`, `/v1/*`, `/healthz` and `/readyz` go to the API, everything else to the PWA, so the session cookie stays first-party. Postgres and Redis stay internal.

## Planned components

| Component | Shape |
|---|---|
| PWA | Separate app after the design system and layout sketch; Web Push for reminders (iOS requires the app to be added to the Home Screen) |
| MCP server | Second driving adapter on the same services, served at `/mcp`, with its own authentik client |
| Reminders & notes | Core entries with `private / shared / reveal_after` visibility; jobs and push delivery via Redis |
| Mealie, Home Assistant, Claude | Driven adapters behind ports; HA actions limited to a whitelist |
| LiveKit calls with a Homeloom agent | Self-hosted LiveKit on Coolify; the API issues room tokens; the agent is a separate service (LiveKit Agents) that calls the API |
| Third-party apps | Per-app authentik clients or tokens, outgoing webhooks, iCal feed for reminders |
