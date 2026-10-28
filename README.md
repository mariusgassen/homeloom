# homeloom

**English** · [Deutsch](README.de.md)

A shared home for two. Homeloom weaves your meals, moments and smart home into one private, self-hosted space for couples and households.

## Repository

- `apps/api`: the Go API (hexagonal, API first). Contract in `apps/api/api/openapi.yaml`.
- `apps/web`: the PWA (Vite, React, TypeScript), installable on iPhone from Safari.
- `docs/architecture.md`: decisions, layout and deployment.
- `docker-compose.yml`: the stack Coolify deploys (`homeloom-web`, `homeloom-api`, `homeloom-postgres`, `homeloom-redis`).

The app speaks German and English. The API answers in the language of the `Accept-Language` header.

## Run locally

```sh
cp .env.example .env   # set POSTGRES_PASSWORD, PUBLIC_URL=http://localhost:3000 and the authentik values
                       # optional: MEALIE_MCP_URL + MEALIE_MCP_TOKEN for cooking, ANTHROPIC_API_KEY for Claude
                       # optional: HOME_ASSISTANT_URL + HOMELOOM_SECRET_KEY for scenes and lights
                       # optional: VAPID_PUBLIC_KEY + VAPID_PRIVATE_KEY for notifications (see .env.example)
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
open http://localhost:3000
```

## Develop the API

```sh
cd apps/api
go generate ./...   # after changing openapi.yaml or SQL
go test ./...       # set TEST_DATABASE_URL and TEST_REDIS_URL to include the adapter tests
```

## Develop the app

```sh
cd apps/web
npm ci
npm run dev    # http://localhost:5173, API calls go to localhost:8080
npm test
```

## Releases

Versions follow SemVer and are released automatically from Conventional Commit PR titles (`feat: …`, `fix: …`). See `CHANGELOG.md` and the images at `ghcr.io/mariusgassen/homeloom-api` and `ghcr.io/mariusgassen/homeloom-web`.
