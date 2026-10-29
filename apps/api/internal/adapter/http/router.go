// Package http is the driving HTTP adapter: it serves the OpenAPI contract on top of the core.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	validator "github.com/oapi-codegen/nethttp-middleware"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

type Deps struct {
	Version        string
	PublicURL      string // where the app is served; calendar profiles point there
	Identity       port.IdentityService
	Login          port.LoginService
	Households     port.HouseholdService
	Questions      port.QuestionService
	Entries        port.EntryService
	Moments        port.MomentService
	Cook           port.CookService
	Shopping       port.ShoppingService
	Home           port.HomeService
	Watch          port.WatchService
	Push           port.PushService
	Calls          port.CallService
	Voice          port.VoiceService
	Avatars        port.AvatarService
	Assistant      port.AssistantService
	Agents         port.AgentAccessService
	HealthChecks   []port.HealthCheck
	AllowedOrigins []string
	// SecureCookies is true when the API is served over HTTPS; SessionTTL is how long a login lasts.
	SecureCookies bool
	SessionTTL    time.Duration
	// MCP, if set, is served at /mcp, outside the OpenAPI contract; it authenticates on its own.
	MCP http.Handler
	// CalDAV, if set, is served at /dav and /.well-known/caldav, outside the contract; it
	// authenticates on its own.
	CalDAV http.Handler
	// Notes, if set, is served at /webdav, outside the contract; it authenticates on its own.
	Notes http.Handler
}

// davMethods are the WebDAV (RFC 4918) and CalDAV (RFC 4791) methods beyond HTTP's own.
var davMethods = []string{"PROPFIND", "PROPPATCH", "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK", "REPORT", "MKCALENDAR"}

// maxBody bounds a request body to the API: a photo and some room.
const maxBody = domain.MaxPhotoBytes + 64<<10

func init() {
	// Photos are sent as they are; the validator only needs to know it may read them.
	openapi3filter.RegisterBodyDecoder("image/jpeg", openapi3filter.FileBodyDecoder)
}

func NewRouter(d Deps) (http.Handler, error) {
	spec, err := oapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}
	spec.Servers = nil

	// chi answers methods it doesn't know with 405, so the WebDAV and CalDAV ones that /dav and
	// /webdav take must be known before those routes are added.
	for _, m := range davMethods {
		chi.RegisterMethod(m)
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, requestLogger, middleware.Recoverer, withLanguage)
	if len(d.AllowedOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins: d.AllowedOrigins,
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Authorization", "Content-Type"},
			MaxAge:         300,
		}))
	}
	if d.MCP != nil {
		r.Handle("/mcp", d.MCP)
	}
	if d.CalDAV != nil {
		r.Handle("/.well-known/caldav", d.CalDAV)
		r.Handle("/dav", d.CalDAV)
		r.Handle("/dav/*", d.CalDAV)
	}
	if d.Notes != nil {
		r.Handle("/webdav", d.Notes)
		r.Handle("/webdav/*", d.Notes)
	}
	// Only the contract's own paths go through the validator, so other paths are answered here.
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(r.Context(), w, http.StatusNotFound, "not_found", "")
	})
	api := r.With(withAuthSlot, limitBody, validator.OapiRequestValidatorWithOptions(spec, &validator.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: authenticate(d.Identity),
			MultiError:         false,
		},
		ErrorHandlerWithOpts: validationError,
	}))

	h := &handlers{
		version:    d.Version,
		publicURL:  d.PublicURL,
		households: d.Households,
		questions:  d.Questions,
		entries:    d.Entries,
		moments:    d.Moments,
		cook:       d.Cook,
		shopping:   d.Shopping,
		home:       d.Home,
		watch:      d.Watch,
		push:       d.Push,
		calls:      d.Calls,
		voice:      d.Voice,
		avatars:    d.Avatars,
		assistant:  d.Assistant,
		agents:     d.Agents,
		login:      d.Login,
		cookies:    cookies{secure: d.SecureCookies, sessionTTL: d.SessionTTL},
		checks:     d.HealthChecks,
	}
	strict := oapi.NewStrictHandlerWithOptions(h, nil, oapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(r.Context(), w, http.StatusBadRequest, "invalid", err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, domain.ErrUnauthenticated) {
				writeError(r.Context(), w, http.StatusUnauthorized, "unauthenticated", "")
				return
			}
			slog.ErrorContext(r.Context(), "request failed", "err", err, "path", logPath(r.URL.Path))
			writeError(r.Context(), w, http.StatusInternalServerError, "internal", "")
		},
	})
	oapi.HandlerWithOptions(strict, oapi.ChiServerOptions{BaseRouter: api})
	return r, nil
}

func validationError(ctx context.Context, err error, w http.ResponseWriter, _ *http.Request, opts validator.ErrorHandlerOpts) {
	if opts.StatusCode == http.StatusUnauthorized {
		if slot := slotFrom(ctx); slot != nil && slot.err != nil && !errors.Is(slot.err, domain.ErrUnauthenticated) {
			// The token could not be checked at all (e.g. authentik unreachable): not the caller's fault.
			slog.ErrorContext(ctx, "authentication unavailable", "err", slot.err)
			writeError(ctx, w, http.StatusServiceUnavailable, "auth_unavailable", "")
			return
		}
		writeError(ctx, w, http.StatusUnauthorized, "unauthenticated", "")
		return
	}
	code := "invalid"
	if opts.StatusCode == http.StatusNotFound {
		code = "not_found"
	}
	writeError(ctx, w, opts.StatusCode, code, err.Error())
}

func writeError(ctx context.Context, w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody(ctx, code, detail))
}

// limitBody keeps a request body from growing without bound before the validator reads it.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}

// withLanguage negotiates the response language once per request; handlers read it with i18n.FromContext.
func withLanguage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := i18n.Negotiate(r.Header.Get("Accept-Language"))
		w.Header().Set("Content-Language", tag.String())
		w.Header().Add("Vary", "Accept-Language")
		next.ServeHTTP(w, r.WithContext(i18n.WithLanguage(r.Context(), tag)))
	})
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.InfoContext(r.Context(), "http",
			"method", r.Method, "path", logPath(r.URL.Path), "status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(), "request_id", middleware.GetReqID(r.Context()))
	})
}

// logPath keeps calendar feed tokens and guest links, which work like passwords, out of the logs.
func logPath(path string) string {
	for _, secret := range []string{calendarFeedPath, guestCallPath} {
		if strings.HasPrefix(path, secret) {
			return secret + "…"
		}
	}
	return path
}
