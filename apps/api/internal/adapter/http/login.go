package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

const (
	sessionCookie = "homeloom_session"
	loginCookie   = "homeloom_login"
	loginPath     = "/auth/callback"
)

// cookies builds the HttpOnly cookies of the browser login. SameSite=Lax keeps them off
// cross-site POSTs (CSRF) while still sending the login cookie on authentik's redirect back.
type cookies struct {
	secure     bool
	sessionTTL time.Duration
}

func (c cookies) make(name, value, path string, maxAge time.Duration) string {
	ck := http.Cookie{ //nolint:gosec // Secure is off only when PUBLIC_URL is plain http (local development)
		Name: name, Value: value, Path: path, MaxAge: int(maxAge.Seconds()),
		HttpOnly: true, Secure: c.secure, SameSite: http.SameSiteLaxMode,
	}
	if maxAge < 0 {
		ck.MaxAge = -1
	}
	return ck.String()
}

func (h *handlers) StartLogin(ctx context.Context, _ oapi.StartLoginRequestObject) (oapi.StartLoginResponseObject, error) {
	state, url, err := h.login.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "login unavailable", "err", err)
		return oapi.StartLogin503JSONResponse{UnavailableJSONResponse: oapi.UnavailableJSONResponse(errorBody(ctx, "auth_unavailable", ""))}, nil
	}
	return oapi.StartLogin302Response{Headers: oapi.StartLogin302ResponseHeaders{
		Location:  url,
		SetCookie: h.cookies.make(loginCookie, state, loginPath, 10*time.Minute),
	}}, nil
}

func (h *handlers) CompleteLogin(ctx context.Context, req oapi.CompleteLoginRequestObject) (oapi.CompleteLoginResponseObject, error) {
	back := func(code string) oapi.CompleteLogin302Response {
		return oapi.CompleteLogin302Response{Headers: oapi.CompleteLogin302ResponseHeaders{Location: "/?auth_error=" + code}}
	}
	if req.Params.Error != nil {
		return back("login_failed"), nil
	}
	sessionID, err := h.login.Complete(ctx, deref(req.Params.State), deref(req.Params.HomeloomLogin), deref(req.Params.Code))
	if errors.Is(err, domain.ErrUnauthenticated) {
		slog.InfoContext(ctx, "login rejected", "err", err)
		return back("login_failed"), nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "login unavailable", "err", err)
		return back("auth_unavailable"), nil
	}
	cookie := h.cookies.make(sessionCookie, sessionID, "/", h.cookies.sessionTTL)
	return oapi.CompleteLogin302Response{Headers: oapi.CompleteLogin302ResponseHeaders{Location: "/", SetCookie: &cookie}}, nil
}

func (h *handlers) Logout(ctx context.Context, req oapi.LogoutRequestObject) (oapi.LogoutResponseObject, error) {
	url, err := h.login.Logout(ctx, deref(req.Params.HomeloomSession))
	if err != nil {
		// The cookie is cleared regardless; the session then expires on its own.
		slog.WarnContext(ctx, "logout incomplete", "err", err)
	}
	return oapi.Logout200JSONResponse{
		Body:    oapi.Logout{RedirectUrl: optional(url)},
		Headers: oapi.Logout200ResponseHeaders{SetCookie: h.cookies.make(sessionCookie, "", "/", -1)},
	}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
