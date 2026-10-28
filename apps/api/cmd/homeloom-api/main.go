// Command homeloom-api is the composition root: it wires adapters to the core and serves HTTP.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpadapter "github.com/mariusgassen/homeloom/apps/api/internal/adapter/http"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/oidc"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/redis"
	"github.com/mariusgassen/homeloom/apps/api/internal/config"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

// version is the release version, set at build time with -ldflags "-X main.version=<version>".
var version = "dev"

func main() {
	// The runtime image has no shell or curl, so the container healthcheck calls the binary itself.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		slog.Error("homeloom-api stopped", "err", err)
		os.Exit(1)
	}
}

func healthcheck() int {
	_, port, err := net.SplitHostPort(os.Getenv("HTTP_ADDR"))
	if err != nil || port == "" {
		port = "8080"
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:" + port + "/healthz") //nolint:gosec // fixed host; the port comes from our own HTTP_ADDR
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}

	rdb, err := redis.Connect(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer rdb.Close()

	store := postgres.NewStore(pool)
	identity := service.NewIdentity(
		oidc.NewVerifier(cfg.OIDCIssuerURL, cfg.OIDCClientIDs),
		redis.NewUserCache(store, rdb, cfg.UserCacheTTL),
	)

	handler, err := httpadapter.NewRouter(httpadapter.Deps{
		Version:        version,
		Identity:       identity,
		Households:     service.NewHousehold(store),
		HealthChecks:   []port.HealthCheck{postgres.HealthCheck{Pool: pool}, redis.HealthCheck{Client: rdb}},
		AllowedOrigins: cfg.AllowedOrigins,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.HTTPAddr, "version", version)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
