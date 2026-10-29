# homeloom

**English** · [Deutsch](README.de.md)

A shared home for two. Homeloom weaves your meals, moments and smart home into one private, self-hosted space for couples and households.

## Repository

- `apps/api`: the Go API (hexagonal, API first). Contract in `apps/api/api/openapi.yaml`.
- `apps/web`: the PWA (Vite, React, TypeScript), installable on iPhone from Safari.
- `apps/agent`: Homeloom's voice in calls (LiveKit Agents for Node).
- `apps/garage`: the S3 store for photos (Garage), set up on start.
- `docs/architecture.md`: decisions, layout and deployment.
- `docker-compose.yml`: the stack Coolify deploys (`homeloom-web`, `homeloom-api`, `homeloom-livekit` for calls, `homeloom-agent`, `homeloom-postgres`, `homeloom-redis`, `homeloom-garage` for photos).

The app speaks German and English. The API answers in the language of the `Accept-Language` header.

## Run locally

```sh
cp .env.example .env   # set POSTGRES_PASSWORD, PUBLIC_URL=http://localhost:3000, the authentik values
                       # and LIVEKIT_API_SECRET (32+ characters) for calls
                       # optional: MEALIE_MCP_URL + MEALIE_MCP_TOKEN for cooking, ANTHROPIC_API_KEY for Claude and the assistant
                       # optional: OPENAI_API_KEY (with ANTHROPIC_API_KEY) to talk to Homeloom in a call
                       #           (background noise is filtered on the agent; VOICE_NOISE_CANCELLATION=off turns it off)
                       # optional: HOME_ASSISTANT_URL + HOMELOOM_SECRET_KEY for scenes and lights
                       # optional: HOMELOOM_SECRET_KEY alone lets each of you sign in to Plex for movies and shows
                       # optional: VAPID_PUBLIC_KEY + VAPID_PRIVATE_KEY for notifications (see .env.example)
                       # optional: S3_ACCESS_KEY_ID + S3_SECRET_ACCESS_KEY to keep photos in homeloom-garage (see .env.example)
                       # optional: TIME_ZONE for reminders and the calendar (Europe/Berlin by default)
                       # optional: OIDC_CLIENT_IDS to let an MCP client such as Claude use PUBLIC_URL/mcp
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
open http://localhost:3000
```

## Calendar and Reminders on iPhone

Homeloom's events and reminders sync both ways with the iPhone's Calendar and Reminders apps over CalDAV. On the iPhone, open Homeloom, go to Settings › Other apps › Calendar and tap Add to iPhone, then install the downloaded profile in Settings (iOS calls it "Not Verified" because it isn't signed). Or make a password instead and add it under Settings › Apps › Calendar › Calendar Accounts › Add Account › Other › Add CalDAV Account, with your Homeloom address as the server and any user name. Each app shows a shared list and one just for you. The password only opens your calendar and reminders, and stops working when you remove it.

## Notes in other apps

Homeloom's notes are also Markdown files over WebDAV, so a notes app or file manager that syncs a WebDAV folder can read and change them (Apple Notes can't). Make a password under Settings › Other apps › Notes and add a WebDAV account in the app with the shown address, any user name and the password. You get a Shared folder and a Private one just for you; "later" notes stay out. Moving a file to the other folder shares it or makes it private.

## Movies and shows

Under Us › Movies & shows, each of you signs in with your own Plex account. Homeloom then shows the movie and series libraries on your own servers and on servers friends share with you, with what you haven't seen yet and a random pick. It only reads: nothing is played or changed, and the Plex token stays on the server, encrypted with `HOMELOOM_SECRET_KEY`. The API must be able to reach your Plex server (on the same network, or through Plex's remote access).

## Photo storage

Photos of moments are kept in Postgres unless you set an S3 key pair. Then they go to `homeloom-garage`, a small S3 store ([Garage](https://garagehq.deuxfleurs.fr)) that runs in the stack and sets itself up on start: it creates the bucket and imports your key. Make the key once, in the format Garage wants:

```sh
S3_ACCESS_KEY_ID=GK$(openssl rand -hex 12)
S3_SECRET_ACCESS_KEY=$(openssl rand -hex 32)
```

Photos from before stay in Postgres and still show; a photo moves to S3 when it's replaced. Garage keeps its data in the `homeloom-garage-data` volume, so back that up along with Postgres. To use another S3-compatible store, also set `S3_ENDPOINT` (and `S3_REGION`, `S3_BUCKET`) and create the bucket and key there yourself.

## Roadmap

Homeloom grows around how you live together. Features are grouped by what you do, not by tech.

### Coming next

**Around the home**
- **Almost home:** a gentle nudge when one of you is on the way ("start the oven? tonight's plan: ...").
- **Weekly Sunday digest:** one page that sums up the week.

**Around the table**
- **Taste profiles:** rate meals, get dinner ideas that suit both of you.
- **Meal memories:** keep photos and notes with the dishes you cook, on your timeline, plus a yearly recap.

**Around the two of you**
- **Time-capsule letters:** write to each other, sealed until a date or milestone.
- **Gratitude wall:** one line a day, resurfaced at random (started).
- **"Us" timeline:** a shared stream of moments, with a monthly digest and "on this day" (started).
- **Gratitude on Today:** write today's line right on Today, with an optional nudge in the evening if you haven't yet.
- **Answers to keep:** put an opened question on your timeline in one tap.
- **Moment reactions:** a heart or a short reply to each other's moments.
- **Shared streaks:** how many days in a row you both kept a line, with no guilt for a missed one.
- **Year in review:** at New Year, a written look back at your moments and thank-yous.
- **Ask about your memories:** the assistant looks things up on your timeline ("what did we do last May?").

**Going out and staying in**
- **Date-night generator:** ideas based on weather, local events, budget and mood.
- **Movie and game night picker:** you each swipe through your Plex libraries, the overlap wins, the room sets itself.

**Connected apps**
- **Plex:** your own and shared movie and series libraries (started, see above).
- **Photos:** add photos from your iPhone to moments and meals, also from the share sheet through a Shortcut. iCloud has no API; public shared albums could be read, but only by making them public.
- **Reading together:** what each of you reads and how far, from a self-hosted BookOrbit.
- **Shared spending:** balances and recent expenses from Tricount, read-only. Tricount has no official API, so this may break.
- **Outgoing hooks and app tokens:** let other apps react to Homeloom (a movie picked, a book finished) and send things in with a token that can do only that.

### Later

- **Dream board:** shared goals, broken into steps, with a quarterly check-in.
- **Perspective helper:** each of you writes your side privately and gets a neutral summary with compromise ideas. Never shows either raw text to the other. Opt-in per entry.
- **Ambient display:** the daily gratitude line on an e-ink screen or smart light.
- **Household assistant:** ask your home a question in plain language.

### Ideas

Not sorted or promised yet.

**Around the home**
- **Chores rotation:** shared chores that take turns fairly.
- **Arrival scenes:** the lights and music you like when you get home, together with Almost home.
- **Plant and house care:** watering, filters, maintenance, reminded at the right time.
- **Energy and home insights:** a calm look at what the home used this week.
- **House wishlist:** things you want for the home, saved and ranked together.
- **Quiet hours:** do-not-disturb for both of you, and the home stays quiet too.
- **Guest and sitter view:** a small, time-limited view for guests or a house sitter.

**Around the table**
- **What's left for dinner:** ideas from what's in the pantry, using up what expires first.

**Around the two of you**
- **Weekly check-in:** a short ritual of a few questions you answer together.
- **Anniversaries and milestones:** the dates that matter, never missed.
- **Gift ideas:** jot down private ideas for the other one, surfaced before birthdays.
- **Voice notes:** leave a short message for each other.
- **Shared location moments:** save the places you've been together.

**Everyday life**
- **Today briefing:** one morning push with the weather, plans and what's due.
- **Budget glance:** a simple view of shared spending.
- **Trips and packing lists:** plan trips and pack from shared lists.
- **Documents vault:** important papers in one private place.
- **Watch, read and visit list:** everything you want to see, read or go to together.

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
