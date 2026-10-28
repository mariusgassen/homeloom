FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/homeloom-api ./cmd/homeloom-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/homeloom-api /homeloom-api
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --retries=3 CMD ["/homeloom-api", "healthcheck"]
ENTRYPOINT ["/homeloom-api"]
