# Homeloom

A private, self-hosted app for a couple or small household: cooking (Mealie), reveal-together questions, reminders, notes and the smart home (Home Assistant), with Claude as the LLM. The person building it is Marius; he often writes in German.

Read `docs/architecture.md` before structural changes.

## Layout

- `apps/api`: Go API, hexagonal, API first. The only component that holds secrets or talks to Mealie, Home Assistant, authentik or the LLM.
- `docs/`: architecture and decisions.
- `docker-compose.yml`: the stack Coolify deploys; `docker-compose.dev.yml` publishes ports locally.
- The PWA (mobile-first, iOS) is not in the repo yet. Its design system and screen layout live as Claude artifacts: design system https://claude.ai/artifact/8eznBKaYPaJu59aiVCnYMi, app layout https://claude.ai/artifact/NnTvP4yyGSMTeugLnM6SRD.

## Commands (run in `apps/api`)

```sh
go generate ./...       # after changing api/openapi.yaml or SQL; regenerates oapi/ and sqlcgen/
go vet ./...
golangci-lint run ./... # config in .golangci.yml
go test -race ./...     # set TEST_DATABASE_URL and TEST_REDIS_URL to include the Postgres/Redis adapter tests
```

CI (`.github/workflows/api.yml`) runs all of the above plus govulncheck and a Docker build; it fails if generated code is stale.

## Rules

**Hexagonal.** `internal/core` (domain, port, service) imports nothing from `internal/adapter`. Adapters implement or call ports and never import each other, except decorators such as `redis.UserCache`. Wiring happens only in `cmd/homeloom-api/main.go`.

**Authorisation lives in `core/service`**, so every driving adapter (HTTP now, MCP and jobs later) gets the same rules. Something a caller may not see is `domain.ErrNotFound`, never a 403, so ids can't be probed.

**API first.** Change `api/openapi.yaml` first, run `go generate`, then implement what the compiler asks for. Never edit `*.gen.go` or `sqlcgen/` by hand. Which routes need auth comes from the spec's `security`, not from code. Handlers only translate between generated types and domain types.

**Database.** Queries are SQL in `internal/adapter/postgres/queries/` compiled by sqlc. Schema changes are new goose files in `migrations/` (`000NN_name.sql`, with `-- +goose Up` and `Down`); never edit a released migration.

**Degrade, don't break.** Mealie, Home Assistant, the LLM, Redis or authentik being down must disable a feature, not the app. Bound every outbound call with a timeout.

**Secrets** only via environment variables (see `.env.example`); never in the repo, logs or the client. Don't log private entry bodies.

**LLM and smart-home actions** go through an explicit whitelist. Home Assistant: lights, scenes, media only; locks, alarms and garage doors never without a human confirmation step.

**Compose services** are prefixed `homeloom-` so they're easy to find in Coolify.

**Use current versions.** New dependencies, images and actions start at their latest stable release; Dependabot keeps them there.

## Localization

German and English from the start; English is the fallback.

- The API negotiates `Accept-Language` per request (`internal/i18n`) and sets `Content-Language`. Error `code`s are stable and never translated; `message` is translated; `detail` is optional English for debugging.
- Server-written text (errors now; notifications and generated text later) uses catalog keys in `internal/i18n/locales/{en,de}.json`. Add every key to both files; a test fails otherwise.
- German copy uses informal "du", short and warm, matching the design system's voice.
- Docs that users read come in both languages: `README.md` and `README.de.md` are kept in sync.

## Versioning and commits

- SemVer, released by release-please from Conventional Commits. PRs are squash-merged, so the **PR title** must be a conventional commit (`feat: …`, `fix: …`, `chore: …`, `feat!: …` for breaking changes); a workflow checks it.
- Merging to `main` updates a release PR; merging that tags `vX.Y.Z`, updates `CHANGELOG.md` and publishes `ghcr.io/mariusgassen/homeloom-api:X.Y.Z`.
- The binary gets its version via `-ldflags "-X main.version=…"` (Docker build arg `VERSION`); `/healthz` reports it.
- `info.version` in `openapi.yaml` is the contract's own version: bump it when the contract changes. Breaking API changes go to a new path prefix (`/v2`), not into `/v1`.
