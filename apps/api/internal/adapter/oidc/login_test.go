package oidc_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/oidc"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func newLogin(is *issuer) *oidc.Login {
	return oidc.NewLogin(oidc.NewProvider(is.srv.URL), "homeloom-web", "s3cret", "https://app.test/auth/callback", "https://app.test/")
}

func TestLoginAuthCodeURL(t *testing.T) {
	is := newIssuer(t)
	raw, err := newLogin(is).AuthCodeURL(context.Background(), "st", "nc", "verifier-verifier-verifier-verifier-verifier")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	for k, want := range map[string]string{
		"client_id": "homeloom-web", "state": "st", "nonce": "nc", "redirect_uri": "https://app.test/auth/callback",
		"code_challenge_method": "S256", "response_type": "code", "scope": "openid profile email",
	} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
	if q.Get("code_challenge") == "" {
		t.Error("no PKCE challenge")
	}
}

func TestLoginExchange(t *testing.T) {
	is := newIssuer(t)
	l := newLogin(is)
	ctx := context.Background()
	var form url.Values
	is.tokenResponse = func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		if user, pass, _ := r.BasicAuth(); user != "homeloom-web" || pass != "s3cret" {
			if r.PostForm.Get("client_secret") != "s3cret" {
				http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
				return
			}
		}
		if r.PostForm.Get("code") != "good" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "token_type": "Bearer", "expires_in": 300,
			"id_token": is.token(t, "homeloom-web", time.Now().Add(time.Hour), map[string]any{"nonce": "nc"}),
		})
	}

	id, idToken, err := l.Exchange(ctx, "good", "the-verifier", "nc")
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "user-1" || id.Name != "Alice" || idToken == "" {
		t.Fatalf("got %+v %q", id, idToken)
	}
	if form.Get("code_verifier") != "the-verifier" {
		t.Errorf("code_verifier = %q", form.Get("code_verifier"))
	}

	if _, _, err := l.Exchange(ctx, "good", "the-verifier", "other-nonce"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("nonce mismatch: want ErrUnauthenticated, got %v", err)
	}
	if _, _, err := l.Exchange(ctx, "bad", "the-verifier", "nc"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("invalid grant: want ErrUnauthenticated, got %v", err)
	}

	is.tokenResponse = func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", http.StatusBadGateway) }
	if _, _, err := l.Exchange(ctx, "good", "the-verifier", "nc"); err == nil || errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("provider error: want an availability error, got %v", err)
	}
}

func TestLoginLogoutURL(t *testing.T) {
	is := newIssuer(t)
	raw, err := newLogin(is).LogoutURL(context.Background(), "the-id-token")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	if u.Path != "/end-session/" || u.Query().Get("id_token_hint") != "the-id-token" || u.Query().Get("post_logout_redirect_uri") != "https://app.test/" {
		t.Fatalf("got %s", raw)
	}
}
