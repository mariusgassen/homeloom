# homeloom

**English** · [Deutsch](README.de.md)

A shared home for two. Homeloom weaves your meals, moments and smart home into one private, self-hosted space for couples and households.

## Repository

- `apps/api`: the Go API (hexagonal, API first). Contract in `apps/api/api/openapi.yaml`.
- `docs/architecture.md`: decisions, layout and deployment.
- `docker-compose.yml`: the stack Coolify deploys (`homeloom-api`, `homeloom-postgres`, `homeloom-redis`).

The app speaks German and English. The API answers in the language of the `Accept-Language` header.

## Run locally

```sh
cp .env.example .env   # set POSTGRES_PASSWORD and the authentik values
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
curl localhost:8080/readyz
```

## Develop the API

```sh
cd apps/api
go generate ./...   # after changing openapi.yaml or SQL
go test ./...       # set TEST_DATABASE_URL and TEST_REDIS_URL to include the adapter tests
```

## Releases

Versions follow SemVer and are released automatically from Conventional Commit PR titles (`feat: …`, `fix: …`). See `CHANGELOG.md` and the images at `ghcr.io/mariusgassen/homeloom-api`.
