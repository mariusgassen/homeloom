# homeloom

[English](README.md) · **Deutsch**

Ein gemeinsames Zuhause für zwei. Homeloom verwebt eure Mahlzeiten, Momente und euer Smart Home zu einem privaten, selbst gehosteten Ort für Paare und Haushalte.

## Repository

- `apps/api`: die Go-API (hexagonal, API first). Der Vertrag liegt in `apps/api/api/openapi.yaml`.
- `apps/web`: die PWA (Vite, React, TypeScript), auf dem iPhone über Safari installierbar.
- `docs/architecture.md`: Entscheidungen, Aufbau und Deployment (auf Englisch).
- `docker-compose.yml`: der Stack, den Coolify ausrollt (`homeloom-web`, `homeloom-api`, `homeloom-postgres`, `homeloom-redis`).

Die App spricht Deutsch und Englisch. Die API antwortet in der Sprache aus dem `Accept-Language`-Header.

## Lokal starten

```sh
cp .env.example .env   # POSTGRES_PASSWORD, PUBLIC_URL=http://localhost:3000 und die authentik-Werte setzen
                       # optional: MEALIE_URL + MEALIE_API_TOKEN zum Kochen, ANTHROPIC_API_KEY für Claude
                       # optional: HOME_ASSISTANT_URL + HOME_ASSISTANT_TOKEN für Szenen und Licht, DINNER_SCENE
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
open http://localhost:3000
```

## An der API entwickeln

```sh
cd apps/api
go generate ./...   # nach Änderungen an openapi.yaml oder SQL
go test ./...       # mit TEST_DATABASE_URL und TEST_REDIS_URL laufen auch die Adapter-Tests
```

## An der App entwickeln

```sh
cd apps/web
npm ci
npm run dev    # http://localhost:5173, API-Aufrufe gehen an localhost:8080
npm test
```

## Releases

Versionen folgen SemVer und werden automatisch aus PR-Titeln im Conventional-Commits-Format veröffentlicht (`feat: …`, `fix: …`). Siehe `CHANGELOG.md` und die Images unter `ghcr.io/mariusgassen/homeloom-api` und `ghcr.io/mariusgassen/homeloom-web`.
