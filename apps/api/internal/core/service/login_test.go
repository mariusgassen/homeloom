package service_test

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func begin(t *testing.T, l *service.Login) string {
	t.Helper()
	state, raw, err := l.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := url.Parse(raw); u.Query().Get("state") != state || len(state) < 43 {
		t.Fatalf("state %q, url %s", state, raw)
	}
	return state
}

func TestLoginFlow(t *testing.T) {
	ctx := context.Background()
	sessions := memory.NewSessions()
	login := service.NewLogin(memory.NewLoginProvider(), sessions)
	identity := service.NewIdentity(nil, sessions, memory.NewStore())

	state := begin(t, login)
	sid, err := login.Complete(ctx, state, state, "code-alice")
	if err != nil {
		t.Fatal(err)
	}
	u, err := identity.AuthenticateSession(ctx, sid)
	if err != nil || u.Name != "alice" {
		t.Fatalf("got %+v, %v", u, err)
	}

	if _, err := login.Complete(ctx, state, state, "code-alice"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("replayed state: want ErrUnauthenticated, got %v", err)
	}

	out, err := login.Logout(ctx, sid)
	if err != nil || out != "https://idp.test/end-session/?id_token_hint=idtoken-alice" {
		t.Fatalf("logout: %q, %v", out, err)
	}
	if _, err := identity.AuthenticateSession(ctx, sid); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("after logout: want ErrUnauthenticated, got %v", err)
	}
	if out, err := login.Logout(ctx, sid); out != "" || err != nil {
		t.Errorf("second logout: %q, %v", out, err)
	}
}

func TestLoginRejects(t *testing.T) {
	ctx := context.Background()
	login := service.NewLogin(memory.NewLoginProvider(), memory.NewSessions())

	for name, tc := range map[string]struct {
		browser func(state string) string
		code    string
	}{
		"other browser": {func(string) string { return "someone-elses-state" }, "code-alice"},
		"no cookie":     {func(string) string { return "" }, "code-alice"},
		"bad code":      {func(s string) string { return s }, "nope"},
	} {
		state := begin(t, login)
		if _, err := login.Complete(ctx, state, tc.browser(state), tc.code); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("%s: want ErrUnauthenticated, got %v", name, err)
		}
	}

	state := begin(t, login)
	if _, err := login.Complete(ctx, state, state, "code-down"); err == nil || errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("provider down: want an availability error, got %v", err)
	}
}
