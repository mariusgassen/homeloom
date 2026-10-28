# homeloom

**English** · [Deutsch](README.de.md)

A shared home for two. Homeloom weaves your meals, moments and smart home into one private, self-hosted space for couples and households.

## Repository

- `apps/api`: the Go API (hexagonal, API first). Contract in `apps/api/api/openapi.yaml`.
- `apps/web`: the PWA (Vite, React, TypeScript), installable on iPhone from Safari.
- `apps/agent`: Homeloom's voice in calls (LiveKit Agents for Node).
- `docs/architecture.md`: decisions, layout and deployment.
- `docker-compose.yml`: the stack Coolify deploys (`homeloom-web`, `homeloom-api`, `homeloom-livekit` for calls, `homeloom-agent`, `homeloom-postgres`, `homeloom-redis`).

The app speaks German and English. The API answers in the language of the `Accept-Language` header.

## Run locally

```sh
cp .env.example .env   # set POSTGRES_PASSWORD, PUBLIC_URL=http://localhost:3000, the authentik values
                       # and LIVEKIT_API_SECRET (32+ characters) for calls
                       # optional: MEALIE_MCP_URL + MEALIE_MCP_TOKEN for cooking, ANTHROPIC_API_KEY for Claude and the assistant
                       # optional: OPENAI_API_KEY (with ANTHROPIC_API_KEY) to talk to Homeloom in a call
                       # optional: HOME_ASSISTANT_URL + HOMELOOM_SECRET_KEY for scenes and lights
                       # optional: VAPID_PUBLIC_KEY + VAPID_PRIVATE_KEY for notifications (see .env.example)
                       # optional: TIME_ZONE for reminders and the calendar (Europe/Berlin by default)
                       # optional: OIDC_CLIENT_IDS to let an MCP client such as Claude use PUBLIC_URL/mcp
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
open http://localhost:3000
```

## Calendar and Reminders on iPhone

Homeloom's events and reminders sync both ways with the iPhone's Calendar and Reminders apps over CalDAV. On the iPhone, open Homeloom, go to Settings › Other apps › Calendar and tap Add to iPhone, then install the downloaded profile in Settings (iOS calls it "Not Verified" because it isn't signed). Or make a password instead and add it under Settings › Apps › Calendar › Calendar Accounts › Add Account › Other › Add CalDAV Account, with your Homeloom address as the server and any user name. Each app shows a shared list and one just for you. The password only opens your calendar and reminders, and stops working when you remove it.

## Notes in other apps

Homeloom's notes are also Markdown files over WebDAV, so a notes app or file manager that syncs a WebDAV folder can read and change them (Apple Notes can't). Make a password under Settings › Other apps › Notes and add a WebDAV account in the app with the shown address, any user name and the password. You get a Shared folder and a Private one just for you; "later" notes stay out. Moving a file to the other folder shares it or makes it private.

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

Versions follow SemVer and are released automatically from Conventional Commit PR titles (`feat: …`, `fix: …`). See `CHANGELOG.md` and the images at `ghcr.io/mariusgassen/homeloom-api`, `ghcr.io/mariusgassen/homeloom-web` and `ghcr.io/mariusgassen/homeloom-agent`.
