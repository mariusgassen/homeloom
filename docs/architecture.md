# Architecture

Homeloom is an API plus separate clients. The API is the only component that holds secrets or talks to Mealie, Home Assistant and the LLM; every client (the PWA, a future iOS app, the MCP server, third-party apps) goes through the same contract and the same permission checks.

```
 PWA ─┐
 iOS ─┼──▶ homeloom-api ──▶ homeloom-postgres
 MCP ─┤    (Go)          ──▶ homeloom-redis
     │                   ──▶ homeloom-garage (S3, photos)
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
- **Other clients (iOS, MCP, third-party):** each is its own authentik provider and sends its access token as `Authorization: Bearer`. The API verifies the JWT against the issuer's JWKS and accepts `OIDC_CLIENT_ID` plus any client ID in `OIDC_CLIENT_IDS` (the compose file passes it through). The provider must have a signing key so tokens are JWTs.
- The spec offers both schemes on every protected operation; either one is enough.
- On the first request a local `users` row is created from the `sub`; later requests are served from the Redis user cache unless the profile changed.
- If authentik or Redis cannot be reached the API returns `503 auth_unavailable`, not `401`, and health checks keep working. A failed login redirects to `/?auth_error=login_failed` or `/?auth_error=auth_unavailable`.
- Sessions are stored under a hash of their id, so a Redis dump holds no usable cookies. They last `SESSION_TTL` (30 days by default).

## Households and questions

- **Invites:** a member creates a one-time code (`POST /v1/households/{id}/invites`), shared as `PUBLIC_URL/join/<code>` and valid for 7 days. Only its SHA-256 is stored. Accepting it (`POST /v1/invites/{code}`) locks the invite row, so a code joins exactly one person. A used, expired or unknown code is the same `404 invite_invalid`. The PWA keeps a pending code in localStorage while the person signs in.
- **Reveal together:** any member asks a question. Answers stay sealed, and the API leaves others' text out, until every current member (at least two) has answered. The last answer reveals the question in the same transaction, under a lock on the question row, so two simultaneous answers cannot both miss the reveal. You can change your answer until then; afterwards it is fixed (`409 already_revealed`).
- Sealing happens in `core/service`, so later adapters (MCP, notifications) cannot leak an answer early.

## Cooking

- **Mealie** (`adapter/mealie`, port `RecipeBook`) holds the recipes and the shopping list. The API reaches it only through Mealie's MCP server (`MEALIE_MCP_URL`, bearer `MEALIE_MCP_TOKEN`), which exposes one tool per Mealie endpoint and holds Mealie's own credentials. The assistant reaches recipes through the API's own tools, not this server. The MCP session is reused and replaced once if the server has forgotten it. Cooking is optional: without `MEALIE_MCP_URL` the cook endpoints answer `503 cooking_unavailable`, and so do they when the MCP server or Mealie is down or refuses a token. Each call is bounded to 8 s. `MEALIE_URL`, if set, is only used for "Open in Mealie" links. **Pictures** come through the same server (`/recipes/{slug}/image`, Mealie's ~600 px rendition): it hands binary answers over as base64 in JSON, and the URL carries the picture's version, so browsers keep it privately for a week.
- **Claude** (`adapter/claude`, port `Chef`) picks up to three recipes for a mood from the whole collection (one short line per recipe, cached as a prompt prefix) and writes why, in the request's language. The answer is constrained to a JSON schema, and the service drops any slug that isn't in the collection. Without `ANTHROPIC_API_KEY`, or when Claude is down, refuses or finds nothing, three recipes are picked at random (`chef: false`). One pick is bounded to 30 s.
- **Claude** also writes reveal-together questions (port `Muse`): five per request, each in the spirit of a randomly drawn theme, unlike the household's recent questions, in the request's language. And it picks films and series (port `Critic`): mostly from the caller's unseen Plex titles (sampled to 150, with 40 seen ones as taste), plus one or two from outside the libraries; the service drops ids it didn't offer. Both answer 503 without Claude; the PWA then falls back to its built-in questions.
- Missing ingredients go to the household's own shopping list (below), noting the recipe they came from. The PWA asks what you already have and sends the rest.
- **Meal plan** (`meal_plan`, port `MealPlanRepository`): one dinner per household and calendar date. Each row keeps the recipe's name, picture version and time, so the plan can be read and changed while Mealie is down; only planning a new recipe and ideas need Mealie. Ideas (`Chef.Plan`) fill only the unplanned days, one different recipe each, leaving out what was planned in the last three weeks unless too few recipes remain. They favour the optional wish and what was ticked off the shopping list (likely in the kitchen). Days Claude leaves open, or all of them without Claude, get random recipes. Ideas are not saved until a member plans them. The week's ingredients go to the list in one call, so the grocer is asked once.

## Shopping list

- One shared list per household (`shopping_items`), in the Lists tab. Every member sees, adds, ticks off and removes the same items; ticked items stay until someone clears them. Mealie's shopping lists are not used.
- The service checks membership, and every query is scoped to the household, so an item id from another household is `404`.
- **Aisles:** each item sits in an aisle (produce, bakery, meat, dairy, frozen, pantry, drinks, household, other) and the list shows them in that order. A new item goes where the household put the same text before (`shopping_aisles`, by lower-cased text, outliving the items), else where Claude says (`adapter/claude`, port `Grocer`, one call per addition for all unknown items, bounded to 8 s), else `other`. Moving an item (`PATCH …/items/{id}`) also updates that memory, so a correction sticks.

## Chores

- `chores` holds what the household does again and again (or once): a title, an effort of 1 to 3 points, a repeat, the day it is due (`due_on`, a date in `TIME_ZONE`) and whose turn it is (`assignee_id`; NULL is anyone's). Chores are shared: every member sees, changes, ticks off and deletes them; the service checks membership and every query is scoped to the household.
- **Ticking off** (`POST …/chores/{id}/done`) records a `chore_completions` row with the chore's title and points, so history and fairness outlive the chore, and how the chore was before. A repeating chore moves to its next day after today or its due day, whichever is later; a one-off gets `done_at` and leaves the list. A rotating chore becomes the turn of the member after whoever did it, in the order they joined. It runs under a lock on the chore row and only if the chore is still as it was read, and once per chore and day (`409 chore_done`), so two taps at once count once.
- **Undo** deletes your own latest completion of a chore and puts the chore back to its day and turn (`409 chore_not_latest` once it was done again).
- **Fairness** (`GET …/chore-fairness`) adds up points and chores per current member for the week from Monday and the month, in `TIME_ZONE`. The history lists the last six weeks.
- **Thanks** (`POST …/chore-completions/{id}/thanks`) is kept once per person (`chore_thanks`) and tells whoever did it. A line sent with it becomes the thanker's gratitude line for today through the Moments service, unless they wrote one already, so a thank-you never replaces what they wrote.
- **Nudge** (`Chores.NotifyDaily`, every minute on each instance, claimed in `job_runs` as `chores-due`): from 18:00, once a day, everyone with a chore due today or overdue hears which ones: their turn, and anyone's.

## Money

- One shared budget per household (`expenses`, `expense_shares`), opened from Today. Every member sees, adds, changes and deletes every expense; the service checks membership and every query is scoped to the household.
- **Expenses** are whole cents in euros, paid by one member, in a category, on a day, and split into shares that add up to the amount, each member at most once. Without shares the service splits evenly between the current members; the first ones carry odd cents.
- **Settling up** is stored as an expense of kind `settlement`: the payer paid it, the receiver has the one share. So where someone stands is one sum over everything, what they paid minus their shares, and settlements need no rules of their own. A settlement can't be changed, only deleted.
- **Month summary:** `GET …/expenses?month=YYYY-MM` also returns the month's `total` and what was spent per category, most first. Settlements count in neither: paying back isn't spending.
- **Receipts** are JPEGs of at most 5 MB kept in S3 only (`receipts/<id>.jpg`, the same `PhotoStore` as photos); Postgres records `receipt_at`. Without S3, or while it is down, receipts answer `503 receipts_unavailable` and the rest of the budget works. Deleting an expense deletes its receipt first.
- **Balance** (`GET …/balance`) lists every member and anyone who left while still owing or being owed, plus the payments that settle it, largest debts first; for two people that is one payment.

## Smart home

- **Home Assistant** (`adapter/homeassistant`, port `SmartHome`) is optional: without `HOME_ASSISTANT_URL` the home endpoints answer `503 home_unavailable`, and so do they when Home Assistant is down. Each call is bounded to 5 s.
- **Each person connects their own account** through Home Assistant's OAuth sign-in, so every switch is made, and logged, as that person. The client id is `PUBLIC_URL/` and the redirect is `PUBLIC_URL/auth/homeassistant/callback`; Home Assistant needs no client registration for that. The state is bound to the person who started the sign-in (Redis, 10 minutes). The refresh token is sealed with AES-GCM under `HOMELOOM_SECRET_KEY` (`adapter/secretbox`, a decorator on the repository) before it reaches Postgres; access tokens live only in the API's memory. When Home Assistant rejects the refresh token, the connection is dropped and the app asks to connect again. Disconnecting revokes the token.
- The whitelist is the port itself: it can list scenes, lights and a few read-only readings (temperature, humidity, power, CO₂; open windows, doors and garage doors, motion, water, smoke), turn a scene on and switch a light. Locks, alarms and covers can't be switched. The contract only accepts `scene.*` and `light.*` ids, the service only switches ids Home Assistant listed, and the adapter checks the domain once more.
- **Rooms** are Home Assistant's areas. The adapter reads the area, device and entity registries over Home Assistant's WebSocket API (the only place they're offered, and readable without admin rights); an entity's room is its own area or its device's. Members of a group helper are left out so the group shows as one control, and so are entities hidden in Home Assistant or marked as config or diagnostic. If the registries can't be read, the home still shows, without rooms.

## Movies and shows

- **Plex** (`adapter/plex`, port `MediaLibrary`) is on when `HOMELOOM_SECRET_KEY` is set, unless `PLEX_ENABLED=false`; otherwise the watch endpoints answer `503 watch_unavailable`. It only reads: libraries, titles and posters. Nothing is played or changed.
- **Each person connects their own account** with Plex's PIN sign-in: the API creates a PIN at plex.tv, sends the browser to app.plex.tv with `forwardUrl` = `PUBLIC_URL/auth/plex/callback?state=…`, and redeems the PIN there. The state is bound to the person (Redis, 10 minutes). The token is sealed under `HOMELOOM_SECRET_KEY` like Home Assistant's, with its own associated data so the two can't be swapped. When plex.tv rejects it, the connection is dropped.
- **Servers** come from plex.tv's resource list: the person's own and those friends share, each with its own access token. The adapter tries every address of a server at once and keeps the most direct that answers (local, then remote, then Plex's relay); a server that can't be reached is left out rather than failing the rest. Lists and addresses are cached for 5 minutes per person. Ids are `<server>.<key>` and only resolve on the caller's own servers, so they can't be probed.
- **Posters** go through the API (`/watch/titles/{id}/poster`, transcoded by Plex to 342×513), so the token never reaches the client.

## PWA

```
apps/web/
  src/api/            typed client; schema.gen.ts is generated from the OpenAPI contract
  src/ui/, styles/    the design system's tokens and components, ported to React
  src/screens/        one file per screen; tabs: Today, Us, Lists, Home; dinner, movies, money and chores open from Today
  src/i18n/           German and English copy
  Caddyfile           static files plus the reverse proxy to the API
```

- The app never sees a token: it starts sign-in with a full navigation to `/auth/login` and then calls the API with the session cookie.
- The service worker precaches the app shell and never handles `/auth`, the health checks or `/v1`, except pictures (avatars, note and moment photos, recipe images): their URLs carry their version, so it keeps them (`homeloom-pictures`, 300 at most, 30 days) and shows them offline.
- **Offline and instant open** (`src/cache.ts`): the API client keeps the last answer to the GETs the app opens with (the session, entries, the shopping list, moments, questions, recipes) in IndexedDB. The app reads them into memory before its first render, so screens show them at once and load again behind them; when the network fails or the API answers 5xx, the client answers from them. The smart home and other live states are never kept. Answers older than 30 days are dropped; a 401, signing out or another person signing in drops everything, pictures too.
- **Changes made offline** (`src/outbox.ts`): creating, editing, ticking off and deleting entries, and every change to the shopping list, wait in an outbox in localStorage when there is no network (or something is waiting already, so order holds). The client answers as the server would, with local ids for what it made, and applies what waits to every GET, so lists show it at once. The outbox is sent in order when the app opens, comes back to the foreground or goes online; a 5xx stops it and it tries again after 30 s. Local ids turn into the real ones, later changes included. Each change belongs to whoever made it and is never sent for someone else; signing out drops it.
- **Conflicts:** an entry edit from the outbox carries the `updatedAt` it was made on as `If-Match` (`PUT /v1/entries/{id}` answers `412 entry_changed` if the entry changed since). Then, or if the entry was deleted meanwhile, the other version stays and the edit is saved as a new entry marked "(offline copy)". Ticks, aisles and deletes are last-write-wins. Several offline edits of one entry are sent as one.
- Installable on iOS from Safari's "Add to Home Screen". Web Push needs that install on iOS; in Safari itself Settings explains how to add the app.

## Calendar, reminders and notes

- One `entries` table holds calendar events, reminders and notes (`kind`), so they share one visibility model: `private` (the author only), `shared` (every member) and `reveal` (notes only: every member sees that it exists, its text opens at `revealAt`). Others' private entries never leave the database; sealing happens in `core/service`. Shared entries can be changed by every member, private and "later" ones only by their author, and only the author decides who sees an entry (`403 not_yours` otherwise).
- **Reminders** have an optional due time, can repeat daily, weekly, monthly or yearly, and can be for one member or everyone. Ticking off a repeating reminder moves it to its next time. Done reminders stay listed for 7 days.
- **Events** have a start, an optional end, can be all day (stored as local midnight of the first and last day), repeat, and have an optional alert minutes before the start. A repeating event is listed once, at its next time.
- **Notifications:** `due_at` is a reminder's due time or an event's next alert. Every API instance runs a loop every 15 s that claims what has come due since `notified_at` (`FOR UPDATE SKIP LOCKED`, so each due time is sent once) and hands it to the Push service: the author for private entries, else the assignee or every member. A time already past when it is set doesn't ring.
- **Time zone:** repeats and all-day events keep their wall-clock time in `TIME_ZONE` (an IANA name, `Europe/Berlin` in the compose file), across daylight saving time.
- **Calendar subscription:** each member can make a private link (`POST /v1/households/{id}/calendar-feed`); only its SHA-256 is stored, a new one replaces the old. `GET /v1/calendar-feeds/{token}` serves an iCalendar feed of the events and dated open reminders that member can see, without a session, for Apple Calendar and others (`webcal://`). Repeats become `RRULE`s in the household's time zone; alerts are left out because Homeloom sends its own. The token never appears in the request log.

- **CalDAV** (`adapter/caldav`, on emersion/go-webdav) serves events and reminders to calendar apps at `/dav`, both ways, so they show in the iPhone's Calendar and Reminders. `/.well-known/caldav` redirects there, so the app needs only the server's name. Each member sees four lists: shared events, their private events, shared reminders and their private reminders (`/dav/<household>/calendars/{events,events-private,reminders,reminders-private}/`). Others' private entries never appear; notes are served by WebDAV below.
  - **Sign-in** is HTTP Basic with a personal calendar password (`hlc_…`), made under Settings › Other apps like the keys for `/mcp` (`mcp_tokens.access = 'calendar'`). The user name is ignored. `POST /v1/households/{id}/calendar-profile` (a form post, so the browser downloads the answer) makes such a password and returns it inside an unsigned Apple configuration profile (`com.apple.caldav.account` for `PUBLIC_URL`), so installing it sets up the account; the password exists only in that file. A password acts as its owner in one household and opens only CalDAV (an `/mcp` key never opens CalDAV). It names its token (`hlc_<id><secret>`) and only a bcrypt hash of the secret is stored; each API instance remembers a password once it has checked out, so a syncing phone doesn't pay for bcrypt on every request, while the token is still looked up each time, so a removed password stops working at once.
  - **The rules are the app's:** `core/service/entry_sync.go` reads and writes through the same checks as the app (`EntrySyncService`). Only the author makes an entry private or shared, which in a calendar app is moving it between lists; the delete that follows such a move is ignored.
  - **Names:** an entry made in Homeloom is `<id>.ics` with UID `<id>@homeloom`; one a calendar app made keeps the app's resource name and UID (`entries.ical_name`, `ical_uid`), so the app finds it where it put it. ETags hash the object.
  - **What carries over:** title, times (all-day too), repeats (daily, weekly, monthly, yearly; intervals, weekdays and ends are dropped), an event's first alert, a reminder's due time and done state. A reminder with only a date is due at 9:00. Changes to single times of a repeat, notes, locations and invitees are not kept. List colours and order are accepted but not stored.
- **Notes over WebDAV** (`adapter/notesdav`, on go-webdav's file server) serves notes as Markdown files at `/webdav`, both ways, for notes apps that sync a folder over WebDAV (Apple Notes can't). There are two fixed folders, `Shared` and `Private` (the member's own); "later" notes never appear, and nothing else can be made: only `.md` and `.txt` files directly in a folder, at most 5,000 characters. It reads and writes through the same `EntrySyncService` as CalDAV, and signs in the same way with its own notes password (`hln_…`, `mcp_tokens.access = 'notes'`), which opens neither CalDAV nor `/mcp`.
  - **Names:** a note an app made keeps the app's file name (`entries.ical_name`); one made in Homeloom is named after its first line, numbered when two share a name, until an app writes it; then it keeps that name. Moving a file renames it, and moving it to the other folder shares it or makes it private (only its author may). An empty file gets its name as its text, since a note needs text. ETags hash the text.

## Moments and gratitude

- One `moments` table holds the Us tab's timeline: **moments** (a line, the day it happened, maybe a photo) and **gratitude lines** (one per person and day; writing again that day replaces the line, enforced by a partial unique index). Both use the entries' visibility model; only the author changes or deletes theirs. `reveal` (sealed until `revealAt`, photo included) is ready for letters to the future; the app doesn't offer it yet.
- `GET /v1/households/{id}/moments?month=YYYY-MM` returns one month, newest day first, and the latest earlier month with anything in it, so the app pages month by month. `…/moments/resurfaced` returns this day in earlier years and one gratitude line older than a week, at random.
- **Photos** are JPEGs of at most 5 MB. With `S3_ACCESS_KEY_ID` and `S3_SECRET_ACCESS_KEY` set they go to S3 (`adapter/s3`, port `PhotoStore`, key `moments/<id>.jpg`; the bundled `homeloom-garage` by default) and Postgres only records `photo_at`. Without them they stay in Postgres (`moment_photos`, apart from the rows so lists never read them). Photos stored in Postgres before S3 was set up are still served from there and move to S3 when replaced. Deleting a photo or its moment deletes the object first, so nothing is left behind; while S3 is down, photos can't be set, shown or deleted, and everything else works. Each S3 call is bounded to 10 s. The PWA makes them at most 1600 px on the long side before upload. `photoUrl` carries the photo's version, so browsers may cache it privately. The API checks the JPEG signature and bounds every request body.
- **Videos** on moments are MP4 or QuickTime files of at most 200 MB, only kept in S3 (port `VideoStore`, key `moments/<id>.video`); without S3 the upload answers 503 (`videos_unavailable`). An upload is streamed through to S3 in 16 MB parts, never held whole: the router lets it skip the request validator's body read and raises its body limit, and the core checks the `ftyp` box and stops the stream past the limit. `GET …/video` answers one `Range` with 206 straight from S3, which iOS needs to play it; `videoUrl` carries the version, like `photoUrl`. Video calls to S3 are bounded to 15 minutes. The PWA first compresses a picked video in the browser (Mediabunny over WebCodecs, loaded on demand) to H.264/AAC MP4 at most 1280 px on the long side, so longer videos fit; where it can't, or the result is not smaller, the video goes as it is. Whether videos play by themselves (muted, while in view) is a per-device setting in the PWA.
- **Daily notifications** (`Moments.NotifyDaily`, every minute on each instance): this day in earlier years at 9:00, last month's look back on the 1st at 19:00, and at 20:00, with a one-in-three chance per person, a gratitude line from more than two weeks ago. Each job claims its day in `job_runs`, so it runs once across instances. Days count in `TIME_ZONE`.

## Assistant

- **Ask Homeloom** is a chat opened from the sparkle in every header. `POST /v1/households/{id}/chat` stores the message and streams the answer back as server-sent events (`conversation`, `tool`, `text`, `done` or `error`). The answer goes on when the browser goes away, so it is there when the person comes back. Each conversation belongs to one person and is visible only to them (`conversations`, `chat_messages`); the assistant reads its last 40 messages.
- **The tools are the whitelist** (`core/service/assistant_tools.go`): list and add reminders, events and notes, tick a reminder off, search and open recipes, read and add to the shopping list, read the home, turn a scene on and switch a light. Nothing deletes. Every tool calls the same service the app does, with the person as the actor, so private entries and unopened notes stay hidden exactly as in the app. Tool input is decoded strictly and validated by the services; a failure goes back to the model as a failed call, and only failures it can fix carry details.
- **Claude** (`adapter/claude`, port `Assistant`) runs the tool loop: up to 10 rounds, each call bounded to 60 s and the whole answer to 2 minutes. The instructions and tools are cached; the time, time zone, the person's name and the other members follow them. It acts when asked and asks first when something is its own idea. Without `ANTHROPIC_API_KEY` the assistant answers `503 assistant_unavailable` and the app says so.
- The same tools are served over MCP (below), so other assistants need no rules of their own.

## MCP server

- `adapter/mcp` serves the assistant's tools at `/mcp` (streamable HTTP, without sessions), outside the OpenAPI contract and next to it in the same router. It runs `ToolService.RunTool`, which checks membership and then calls the same tools as the chat, so a caller sees and changes exactly what they could in the app. It needs no LLM: the client brings its own model.
- **Who it acts as** comes from the bearer token on every request:
  - a **personal key** (`hlp_…`) a member makes under Settings › Other apps for one app (`/v1/households/{id}/mcp-tokens`). It acts as them in that household until they remove it. Only its SHA-256 is stored (`mcp_tokens`), and the key is shown once.
  - an **access token from authentik**, for MCP clients such as Claude. Give the client its own authentik provider and add its client ID to `OIDC_CLIENT_IDS`. `?household=<id>` picks the household; members of just one may leave it out.
  - an **agent token** (`hla_…`) for the voice agent. `AgentAccessService.Mint` makes one for a member and one household, lasting up to 6 hours, and `Revoke` ends it, for example when a call ends. Only a hash of the token is stored, in Redis, until it expires. It is never sent to authentik.
- A missing or unknown token gets `401` with `WWW-Authenticate`; authentik being unreachable gets `503`.

## Notifications

- **Web Push with VAPID** (`adapter/webpush`, port `Pusher`) is optional: without `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY`, `GET /v1/push` answers `503 push_unavailable` and the PWA hides the switch. `homeloom-api vapid-keys` prints a new pair. `VAPID_SUBJECT` defaults to `PUBLIC_URL`.
- Subscriptions are per person and device (`push_subscriptions`, keyed by endpoint), each with the language it was made in, so every device gets its notification in its own language from the server's catalog. Only the push services of Apple, Google, Mozilla and Microsoft are accepted as endpoints, because the API posts to them. One that the push service reports gone (404/410) is deleted.
- Turning it on is a tap in Settings: the permission prompt comes first (iOS only shows it for a user gesture), then `PushManager.subscribe` with the server's key. The service worker (`public/push-sw.js`, imported into the generated one) shows the notification and opens its page when tapped.
- The core decides who hears what. Notifications are sent in the background, bounded to 10 s per push service and 30 s per round, and never fail the request that caused them; on shutdown the API waits for those under way.
- When the last member answers a question, everyone else hears that the answers are open. Reminders and event alerts notify when they come due (see above); chores nudge in the evening and pass on thanks (see Chores).

## Calls

- **LiveKit** (`homeloom-livekit`, self-hosted) carries voice and video calls. The API (`adapter/livekit`, port `CallServer`) signs LiveKit access tokens with `LIVEKIT_API_KEY`/`LIVEKIT_API_SECRET` and asks LiveKit's room service who is in a room. Without the key pair calls are off and the call endpoints answer `503 calls_unavailable`; so they do when LiveKit is down (bounded to 5 s).
- **A call** is a room named after its household and a new id, `call-<household>-<call>`, so a call id can't open another household's room; LiveKit creates it when the first person joins and closes it a minute after the last one leaves. Nothing about calls is stored in Postgres.
- **End-to-end encryption:** every call is end-to-end encrypted (LiveKit E2EE, a shared key per room), on top of DTLS-SRTP to LiveKit. The API makes a random key for a room the first time someone joins, keeps it in Redis (`CallKey`, 24 hours after the last join) and returns it as `e2eeKey` with every token for that room: members, guests and the voice agent (in its job metadata) get the same one. LiveKit never gets the key of a call between people, so it forwards audio and video it can't read; the API, which hands out the key, could. A call with Homeloom is the exception: the agent's job metadata travels through LiveKit's dispatch, so LiveKit could read that key, just as it could the agent's `hla_` token. The PWA encrypts in LiveKit's worker before anything leaves the phone (after the blur and mask effects), and a browser without encryption support doesn't join.
- **Members:** `POST /v1/households/{id}/calls` starts a call and rings the members in `invite`; `…/calls/{callId}/join` joins or answers it; `…/calls/{callId}/invites` rings more members, skipping those already in it. `core/service` checks that caller and invitees are members. A token is good for 10 minutes to connect; LiveKit refreshes it for a connected client. The push opens `/call/<callId>?from=<id>`, where answering is a tap, because iOS only plays a call's audio when it starts in a gesture.
- **Guests:** a member makes a guest link for a call (`…/calls/{callId}/guest-links`), shared as `PUBLIC_URL/guest/<token>`. Only the token's SHA-256 is stored, in Redis, for 12 hours. The link only reaches that one call and only one guest: the first device that joins with it claims it (a random id the browser keeps for the link; Redis keeps its hash), so a forwarded link answers `409 guest_link_used`, while the guest can come back if their connection drops. The guest page (`GET`/`POST /v1/guest-calls/{token}`, no sign-in) shows who invited them and joins under the name they type, with a fresh `guest-<uuid>` identity each time, so a guest can never pass for a member. The token never appears in the request log.
- **Network:** clients connect to `wss://PUBLIC_URL/rtc`; `homeloom-web`'s Caddy forwards `/rtc` to LiveKit, so there is no second domain or certificate. Audio and video go straight to LiveKit on UDP 7882 (TCP 7881 where UDP is blocked) on the server's public IP, which LiveKit finds by STUN. Both ports are published by the compose file and must be open in the firewall. There is no TURN server yet, so a network that blocks both won't connect.
- **In the PWA**, the phone button on the Us tab calls the other member, or asks who to call when there are more. The call shows over every tab as a grid of people, with mute, camera, add people (ring members or make a guest link) and hang up. It ends for you when you hang up, when everyone else has left, or when nobody joins within 45 s.
- **Talking to Homeloom:** the phone in the assistant sheet calls `POST /v1/households/{id}/calls/homeloom`. The API makes a room only the caller is in (`homeloom-<household>-<user>-<id>`, so nobody can be added to it), mints an agent token (`hla_…`, 2 hours) for the caller and that household, and asks LiveKit's agent dispatch to send `homeloom-agent` there with `{key, e2ee, lang, name, voice}` as job metadata. The key goes from the API to LiveKit to the agent and never reaches the client. The agent (`apps/agent`, LiveKit Agents for Node) hears with OpenAI `gpt-4o-transcribe`, thinks with Claude (`VOICE_LLM_MODEL`), speaks with `gpt-4o-mini-tts`, uses Silero to tell when someone is speaking, and gets the assistant's tools from `http://homeloom-api:8080/mcp` with the key, so it sees and does only what the caller may. It answers in the caller's app language and greets them first.
- **Homeloom's voice:** each member picks how Homeloom sounds on their calls in Settings (`GET`/`PUT /v1/me/voice`, table `voice_settings`, defaults until saved): a `gpt-4o-mini-tts` voice, a speed (0.75 to 1.5) and a tone (warm, calm, playful, clear). `domain.VoiceTone` turns a tone into the model's `instructions`, and the API sends voice, speed and instructions in the dispatch's `voice`; settings that can't be read fall back to the defaults rather than stopping the call. `POST /v1/me/voice/preview` speaks a short sentence with unsaved settings through `adapter/openai` (port `Speaker`, 20 s bound), so the preview sounds like the call; it answers `503 voice_unavailable` when the voice agent is off. The call shows the agent as an orb (`AssistantOrb`) that follows its `lk.agent.state` attribute and the audio level of whoever is talking. Without `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` the API switches the voice agent off, the endpoint answers `503 calls_unavailable`, and the agent container idles.

## Failure handling

- `/healthz` reports the process is up; `/readyz` checks Postgres and Redis and returns `503` when one is down.
- Redis outages degrade to direct database access; each Redis call is bounded to 250 ms.
- Mealie, Home Assistant, the LLM, Web Push, LiveKit and S3 follow the same rule: their outage disables a feature, never the app.

## Deployment on Coolify

- Create a Docker Compose resource from this repository (`docker-compose.yml`).
- Set `POSTGRES_PASSWORD`, `PUBLIC_URL`, `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` and `LIVEKIT_API_SECRET` (at least 32 characters).
- In authentik, register `PUBLIC_URL/auth/callback` as the redirect URI and `PUBLIC_URL/` as the post-logout redirect.
- Assign the public domain to `homeloom-web` (port 8080). Its Caddy serves the PWA and forwards `/auth/*`, `/v1/*`, `/mcp`, `/healthz` and `/readyz` to `homeloom-api`, so app and API share one origin and the session cookie stays first-party, and `/rtc` to `homeloom-livekit`. The API, Postgres and Redis stay internal.
- Open UDP 7882 and TCP 7881 in the server's firewall for call audio and video.
- Optional: set `OPENAI_API_KEY` (and `ANTHROPIC_API_KEY`) to talk to Homeloom in a call.
- Optional: set `S3_ACCESS_KEY_ID` (`GK` + 24 hex characters) and `S3_SECRET_ACCESS_KEY` (64 hex characters) to keep photos in `homeloom-garage`. Its entrypoint makes a single-node layout, the bucket `S3_BUCKET` and imports the key, every start and idempotently; the RPC secret is generated once into its volume. Back up `homeloom-garage-data` with Postgres.

## Planned components

| Component | Shape |
|---|---|
| Home Assistant media | Media players join scenes and lights in the whitelist |
| Third-party apps | Per-app authentik clients or tokens, outgoing webhooks |
