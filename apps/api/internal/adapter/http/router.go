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
	Identity       port.IdentityService
	Login          port.LoginService
	Households     port.HouseholdService
	Questions      port.QuestionService
	Entries        port.EntryService
	Cook           port.CookService
	Home           port.HomeService
	Push           port.PushService
	Calls          port.CallService
	HealthChecks   []port.HealthCheck
	AllowedOrigins []string
	// SecureCookies is true when the API is served over HTTPS; SessionTTL is how long a login lasts.
	SecureCookies bool
	SessionTTL    time.Duration
}

func NewRouter(d Deps) (http.Handler, error) {
	spec, err := oapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}
	spec.Servers = nil

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
	r.Use(withAuthSlot)
	r.Use(validator.OapiRequestValidatorWithOptions(spec, &validator.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: authenticate(d.Identity),
			MultiError:         false,
		},
		ErrorHandlerWithOpts: validationError,
	}))

	h := &handlers{
		version:    d.Version,
		households: d.Households,
		questions:  d.Questions,
		entries:    d.Entries,
		cook:       d.Cook,
		home:       d.Home,
		push:       d.Push,
		calls:      d.Calls,
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
	return oapi.HandlerWithOptions(strict, oapi.ChiServerOptions{BaseRouter: r}), nil
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

// logPath keeps calendar feed tokens, which work like passwords, out of the logs.
func logPath(path string) string {
	if strings.HasPrefix(path, calendarFeedPath) {
		return calendarFeedPath + "…"
	}
	return path
}
