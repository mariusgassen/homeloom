// Command homeloom-api is the composition root: it wires adapters to the core and serves HTTP.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // TIME_ZONE works in the distroless image

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/caldav"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/claude"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/homeassistant"
	httpadapter "github.com/mariusgassen/homeloom/apps/api/internal/adapter/http"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/livekit"
	mcpadapter "github.com/mariusgassen/homeloom/apps/api/internal/adapter/mcp"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/mealie"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/notesdav"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/oidc"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/openai"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/plex"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/redis"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/secretbox"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/webpush"
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
	// Prints a VAPID key pair to set as VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY.
	if len(os.Args) > 1 && os.Args[1] == "vapid-keys" {
		os.Exit(vapidKeys())
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

func vapidKeys() int {
	pub, priv, err := webpush.GenerateKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("VAPID_PUBLIC_KEY=%s\nVAPID_PRIVATE_KEY=%s\n", pub, priv)
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
	sessions := redis.NewSessions(rdb, cfg.SessionTTL)
	provider := oidc.NewProvider(cfg.OIDCIssuerURL)
	users := redis.NewUserCache(store, rdb, cfg.UserCacheTTL)
	identity := service.NewIdentity(
		oidc.NewVerifier(provider, append([]string{cfg.OIDCClientID}, cfg.OIDCClientIDs...)),
		sessions,
		users,
	)
	login := service.NewLogin(
		oidc.NewLogin(provider, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.RedirectURL(), cfg.PostLogoutURL()),
		sessions,
	)

	// Mealie, Claude, Home Assistant, Plex, Web Push and LiveKit are optional; a nil port switches the feature off or to its fallback.
	var book port.RecipeBook
	if cfg.MealieMCPURL != "" {
		if book, err = mealie.New(cfg.MealieMCPURL, cfg.MealieMCPToken, cfg.MealieURL); err != nil {
			return err
		}
	}
	var chef port.Chef
	var llm port.Assistant
	if cfg.AnthropicAPIKey != "" {
		chef = claude.NewChef(cfg.AnthropicAPIKey, cfg.LLMModel)
		llm = claude.NewAssistant(cfg.AnthropicAPIKey, cfg.LLMModel)
	}
	// Tokens from signing in to Home Assistant or Plex are sealed with HOMELOOM_SECRET_KEY.
	var box *secretbox.Box
	if cfg.SecretKey != "" {
		if box, err = secretbox.New(cfg.SecretKey); err != nil {
			return err
		}
	}
	var home port.SmartHome
	var homeConnections port.HomeConnectionRepository = store
	if cfg.HomeAssistantURL != "" {
		if home, err = homeassistant.New(cfg.HomeAssistantURL, cfg.PublicURL); err != nil {
			return err
		}
		homeConnections = secretbox.NewHomeConnections(store, box)
	}
	var media port.MediaLibrary
	var watchConnections port.WatchConnectionRepository = store
	if cfg.Plex {
		media = plex.New(cfg.PublicURL)
		watchConnections = secretbox.NewWatchConnections(store, box)
	}
	var pusher port.Pusher
	if cfg.VAPIDPublicKey != "" {
		if pusher, err = webpush.New(cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey, cfg.VAPIDSubject); err != nil {
			return err
		}
	}
	push := service.NewPush(store, pusher)
	var calls port.CallServer
	if cfg.LiveKitAPIKey != "" {
		if calls, err = livekit.New(cfg.LiveKitPublicURL(), cfg.LiveKitURL, cfg.LiveKitAPIKey, cfg.LiveKitAPISecret); err != nil {
			return err
		}
	}
	// Homeloom speaks only where it can be called: with the voice agent set up.
	var speaker port.Speaker
	if cfg.VoiceAgent != "" {
		speaker = openai.NewSpeaker(cfg.OpenAIAPIKey)
	}
	voice := service.NewVoice(store, speaker)
	// Notifications still under way get to finish after the server has stopped taking requests.
	defer push.Wait()

	entries := service.NewEntry(store, store, store, push, cfg.TimeZone)
	shopping := service.NewShopping(store, store)
	go notifyDue(ctx, entries)
	moments := service.NewMoments(store, store, store, push, cfg.TimeZone)
	go notifyDaily(ctx, moments)
	cook := service.NewCook(store, book, chef, shopping)
	homeService := service.NewHome(store, homeConnections, sessions, home)
	households := service.NewHousehold(store)
	watch := service.NewWatch(store, watchConnections, sessions, media)
	assistant := service.NewAssistant(store, store, llm, entries, cook, shopping, homeService, cfg.TimeZone)
	agents := service.NewAgentAccess(store, sessions, store)

	handler, err := httpadapter.NewRouter(httpadapter.Deps{
		Version:        version,
		PublicURL:      cfg.PublicURL,
		Identity:       identity,
		Login:          login,
		Households:     households,
		Questions:      service.NewQuestion(store, store, push),
		Entries:        entries,
		Moments:        moments,
		Cook:           cook,
		Shopping:       shopping,
		Home:           homeService,
		Watch:          watch,
		Push:           push,
		Calls:          service.NewCall(store, sessions, calls, push).WithVoiceAgent(cfg.VoiceAgent, agents).WithVoice(voice),
		Voice:          voice,
		Avatars:        service.NewAvatars(users),
		Assistant:      assistant,
		Agents:         agents,
		HealthChecks:   []port.HealthCheck{postgres.HealthCheck{Pool: pool}, redis.HealthCheck{Client: rdb}},
		AllowedOrigins: cfg.AllowedOrigins,
		SecureCookies:  cfg.SecureCookies(),
		SessionTTL:     cfg.SessionTTL,
		MCP: mcpadapter.NewHandler(mcpadapter.Deps{
			Version:    version,
			Identity:   identity,
			Agents:     agents,
			Households: households,
			Tools:      assistant,
		}),
		CalDAV: caldav.NewHandler(caldav.Deps{Agents: agents, Entries: entries, Location: cfg.TimeZone}),
		Notes:  notesdav.NewHandler(notesdav.Deps{Agents: agents, Entries: entries}),
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

// notifyDue sends due reminders and event alerts until ctx ends. Several API instances may run
// it: each due time is claimed by one.
func notifyDue(ctx context.Context, entries *service.Entry) {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		for {
			// A full batch means more may be waiting.
			n, err := entries.NotifyDue(ctx)
			if err != nil {
				slog.WarnContext(ctx, "notify due entries", "err", err)
			}
			if n < service.DueBatch {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// notifyDaily sends the timeline's daily notifications until ctx ends. Several API instances may
// run it: each goes out once a day.
func notifyDaily(ctx context.Context, moments *service.Moments) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		if err := moments.NotifyDaily(ctx); err != nil {
			slog.WarnContext(ctx, "notify daily moments", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
