FROM node:lts-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# Rebuild Caddy with Go 1.27 and patched Go modules so the image stays ahead of upstream Caddy releases.
FROM golang:1.27-alpine AS caddy-build
WORKDIR /src
RUN printf 'package main\n\nimport (\n\t_ "time/tzdata"\n\n\tcaddycmd "github.com/caddyserver/caddy/v2/cmd"\n\t_ "github.com/caddyserver/caddy/v2/modules/standard"\n)\n\nfunc main() {\n\tcaddycmd.Main()\n}\n' > main.go && \
  go mod init caddy && \
  go get \
    github.com/caddyserver/caddy/v2@v2.11.9 \
    github.com/klauspost/compress@v1.20.5 \
    golang.org/x/net@v0.63.0 \
    go.opentelemetry.io/otel@v1.47.0 \
    go.opentelemetry.io/otel/sdk@v1.47.0 \
    go.opentelemetry.io/otel/sdk/log@v1.47.0 \
    go.opentelemetry.io/otel/trace@v1.47.0 && \
  go mod tidy && \
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/caddy .

FROM caddy:2-alpine
COPY --from=caddy-build /out/caddy /usr/bin/caddy
RUN setcap cap_net_bind_service=+p /usr/bin/caddy && chmod +x /usr/bin/caddy
COPY Caddyfile /etc/caddy/Caddyfile
COPY --from=build /src/dist /srv
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/manifest.webmanifest >/dev/null || exit 1
