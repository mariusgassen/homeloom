// Package http is the driving HTTP adapter: it serves the OpenAPI contract on top of the core.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	validator "github.com/oapi-codegen/nethttp-middleware"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

type Deps struct {
	Identity       port.IdentityService
	Households     port.HouseholdService
	HealthChecks   []port.HealthCheck
	AllowedOrigins []string
}

func NewRouter(d Deps) (http.Handler, error) {
	spec, err := oapi.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}
	spec.Servers = nil

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, requestLogger, middleware.Recoverer)
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

	strict := oapi.NewStrictHandlerWithOptions(&handlers{households: d.Households, checks: d.HealthChecks}, nil, oapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, http.StatusBadRequest, "invalid", err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, domain.ErrUnauthenticated) {
				writeError(w, http.StatusUnauthorized, "unauthenticated", "missing or invalid access token")
				return
			}
			slog.ErrorContext(r.Context(), "request failed", "err", err, "path", r.URL.Path)
			writeError(w, http.StatusInternalServerError, "internal", "something went wrong")
		},
	})
	return oapi.HandlerWithOptions(strict, oapi.ChiServerOptions{BaseRouter: r}), nil
}

func validationError(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, opts validator.ErrorHandlerOpts) {
	if opts.StatusCode == http.StatusUnauthorized {
		if slot := slotFrom(ctx); slot != nil && slot.err != nil && !errors.Is(slot.err, domain.ErrUnauthenticated) {
			// The token could not be checked at all (e.g. authentik unreachable): not the caller's fault.
			slog.ErrorContext(ctx, "authentication unavailable", "err", slot.err)
			writeError(w, http.StatusServiceUnavailable, "auth_unavailable", "sign-in is temporarily unavailable")
			return
		}
		writeError(w, http.StatusUnauthorized, "unauthenticated", "missing or invalid access token")
		return
	}
	code := "invalid"
	if opts.StatusCode == http.StatusNotFound {
		code = "not_found"
	}
	writeError(w, opts.StatusCode, code, err.Error())
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(oapi.Error{Code: code, Message: msg})
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.InfoContext(r.Context(), "http",
			"method", r.Method, "path", r.URL.Path, "status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(), "request_id", middleware.GetReqID(r.Context()))
	})
}
