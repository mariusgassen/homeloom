package homeassistant_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/homeassistant"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

const states = `[
  {"entity_id": "light.kitchen", "state": "on", "attributes": {"friendly_name": "Küche"}},
  {"entity_id": "light.hall", "state": "unavailable", "attributes": {}},
  {"entity_id": "scene.dinner", "state": "2026-10-28T18:00:00+00:00", "attributes": {"friendly_name": "Abendessen"}},
  {"entity_id": "lock.front_door", "state": "locked", "attributes": {"friendly_name": "Haustür"}},
  {"entity_id": "alarm_control_panel.home", "state": "armed_away", "attributes": {}}
]`

const app = "https://homeloom.test"

// fake is a Home Assistant that hands out the access token "access-<n>" for the code "good"
// and the refresh token "refresh".
type fake struct {
	calls    []string // "<service> <entity_id>"
	grants   []string // grant types asked for at the token endpoint
	revoked  []string
	issued   int
	down     bool
	rejected bool // the refresh token was revoked
}

func (f *fake) server(t *testing.T) *httptest.Server {
	t.Helper()
	api := http.NewServeMux()
	api.HandleFunc("GET /api/states", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(states)) })
	api.HandleFunc("GET /api/states/light.kitchen", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"entity_id": "light.kitchen", "state": "on", "attributes": {"friendly_name": "Küche"}}`))
	})
	api.HandleFunc("POST /api/services/{domain}/{service}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			EntityID string `json:"entity_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.calls = append(f.calls, r.PathValue("domain")+"."+r.PathValue("service")+" "+body.EntityID)
		_, _ = w.Write([]byte(`[]`))
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.grants = append(f.grants, r.PostForm.Get("grant_type"))
		if r.PostForm.Get("client_id") != app+"/" {
			t.Errorf("client_id %q", r.PostForm.Get("client_id"))
		}
		ok := (r.PostForm.Get("grant_type") == "authorization_code" && r.PostForm.Get("code") == "good") ||
			(r.PostForm.Get("grant_type") == "refresh_token" && r.PostForm.Get("refresh_token") == "refresh" && !f.rejected)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "invalid_grant"}`))
			return
		}
		f.issued++
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access-" + strconv.Itoa(f.issued), "expires_in": 1800, "refresh_token": "refresh", "token_type": "Bearer"})
	})
	mux.HandleFunc("POST /auth/revoke", func(_ http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.revoked = append(f.revoked, r.PostForm.Get("token"))
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if f.down {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if r.Header.Get("Authorization") != "Bearer access-"+strconv.Itoa(f.issued) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		api.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func signIn(t *testing.T, f *fake) (*homeassistant.Client, string) {
	t.Helper()
	c, err := homeassistant.New(f.server(t).URL, app)
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := c.Exchange(context.Background(), "good")
	if err != nil || refresh != "refresh" {
		t.Fatalf("exchange: %q, %v", refresh, err)
	}
	return c, refresh
}

func TestAuthorizeURL(t *testing.T) {
	c, _ := homeassistant.New("https://ha.test/", app)
	u, _ := url.Parse(c.AuthorizeURL("st"))
	q := u.Query()
	if u.Host != "ha.test" || u.Path != "/auth/authorize" || q.Get("client_id") != app+"/" ||
		q.Get("redirect_uri") != app+"/auth/homeassistant/callback" || q.Get("state") != "st" || q.Get("response_type") != "code" {
		t.Fatalf("got %s", u)
	}
}

func TestDevicesAreOnlyScenesAndLights(t *testing.T) {
	c, refresh := signIn(t, &fake{})
	scenes, lights, err := c.Devices(context.Background(), refresh)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(scenes, []domain.Scene{{ID: "scene.dinner", Name: "Abendessen"}}) ||
		!slices.Equal(lights, []domain.Light{{ID: "light.kitchen", Name: "Küche", On: true}}) {
		t.Fatalf("got %v %v", scenes, lights)
	}
}

func TestSwitching(t *testing.T) {
	ctx := context.Background()
	f := &fake{}
	c, refresh := signIn(t, f)

	if err := c.ActivateScene(ctx, refresh, "scene.dinner"); err != nil {
		t.Fatal(err)
	}
	l, err := c.ToggleLight(ctx, refresh, "light.kitchen")
	if err != nil || l.On || l.Name != "Küche" {
		t.Fatalf("toggle: %+v, %v", l, err)
	}
	if err := c.ActivateScene(ctx, refresh, "lock.front_door"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("lock as scene: %v", err)
	}
	if _, err := c.ToggleLight(ctx, refresh, "alarm_control_panel.home"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("alarm as light: %v", err)
	}
	if !slices.Equal(f.calls, []string{"scene.turn_on scene.dinner", "light.turn_off light.kitchen"}) {
		t.Fatalf("calls %v", f.calls)
	}
	// The access token from the sign-in is reused until it runs out.
	if !slices.Equal(f.grants, []string{"authorization_code"}) {
		t.Fatalf("grants %v", f.grants)
	}
}

func TestTokens(t *testing.T) {
	ctx := context.Background()
	f := &fake{}
	c, refresh := signIn(t, f)

	// A refused access token is dropped; the next call refreshes it.
	f.issued++
	if _, _, err := c.Devices(ctx, refresh); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("stale access token: %v", err)
	}
	if _, _, err := c.Devices(ctx, refresh); err != nil {
		t.Fatalf("after refresh: %v", err)
	}
	if !slices.Equal(f.grants, []string{"authorization_code", "refresh_token"}) {
		t.Fatalf("grants %v", f.grants)
	}

	if _, err := c.Exchange(ctx, "bad"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("bad code: %v", err)
	}
	if err := c.Revoke(ctx, refresh); err != nil || !slices.Equal(f.revoked, []string{"refresh"}) {
		t.Fatalf("revoke: %v %v", f.revoked, err)
	}
	f.rejected = true
	if _, _, err := c.Devices(ctx, refresh); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("revoked refresh token: %v", err)
	}
}

func TestUnavailable(t *testing.T) {
	f := &fake{}
	c, refresh := signIn(t, f)
	f.down = true
	if _, _, err := c.Devices(context.Background(), refresh); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("down: %v", err)
	}
	gone, _ := homeassistant.New("http://127.0.0.1:1", app)
	if _, err := gone.Exchange(context.Background(), "good"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("unreachable: %v", err)
	}
	if _, err := homeassistant.New("ftp://ha", app); err == nil {
		t.Fatal("accepted a non-http URL")
	}
}
