package homeassistant_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
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

type fake struct {
	calls []string // "<service> <entity_id>"
	down  bool
}

func (f *fake) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/states", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(states)) })
	mux.HandleFunc("GET /api/states/light.kitchen", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"entity_id": "light.kitchen", "state": "on", "attributes": {"friendly_name": "Küche"}}`))
	})
	mux.HandleFunc("POST /api/services/{domain}/{service}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			EntityID string `json:"entity_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.calls = append(f.calls, r.PathValue("domain")+"."+r.PathValue("service")+" "+body.EntityID)
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.down {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDevicesAreOnlyScenesAndLights(t *testing.T) {
	f := &fake{}
	c, err := homeassistant.New(f.server(t).URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	scenes, lights, err := c.Devices(context.Background())
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
	c, _ := homeassistant.New(f.server(t).URL, "tok")

	if err := c.ActivateScene(ctx, "scene.dinner"); err != nil {
		t.Fatal(err)
	}
	l, err := c.ToggleLight(ctx, "light.kitchen")
	if err != nil || l.On || l.Name != "Küche" {
		t.Fatalf("toggle: %+v, %v", l, err)
	}
	if err := c.ActivateScene(ctx, "lock.front_door"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("lock as scene: %v", err)
	}
	if _, err := c.ToggleLight(ctx, "alarm_control_panel.home"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("alarm as light: %v", err)
	}
	if !slices.Equal(f.calls, []string{"scene.turn_on scene.dinner", "light.turn_off light.kitchen"}) {
		t.Fatalf("calls %v", f.calls)
	}
}

func TestUnavailable(t *testing.T) {
	ctx := context.Background()
	f := &fake{down: true}
	srv := f.server(t)
	c, _ := homeassistant.New(srv.URL, "tok")
	if _, _, err := c.Devices(ctx); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("down: %v", err)
	}
	c, _ = homeassistant.New(srv.URL, "wrong")
	f.down = false
	if err := c.ActivateScene(ctx, "scene.dinner"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("bad token: %v", err)
	}
	if _, err := homeassistant.New("ftp://ha", "tok"); err == nil {
		t.Fatal("accepted a non-http URL")
	}
}
