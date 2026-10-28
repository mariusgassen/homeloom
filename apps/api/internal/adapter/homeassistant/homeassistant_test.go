package homeassistant_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/homeassistant"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

const states = `[
  {"entity_id": "light.kitchen", "state": "on", "attributes": {"friendly_name": "Küche"}},
  {"entity_id": "light.hall", "state": "unavailable", "attributes": {}},
  {"entity_id": "light.living", "state": "off", "attributes": {"friendly_name": "Wohnzimmer", "entity_id": ["light.sofa", "light.shelf"]}},
  {"entity_id": "light.sofa", "state": "off", "attributes": {"friendly_name": "Sofa"}},
  {"entity_id": "light.shelf", "state": "off", "attributes": {"friendly_name": "Regal"}},
  {"entity_id": "scene.dinner", "state": "2026-10-28T18:00:00+00:00", "attributes": {"friendly_name": "Abendessen", "entity_id": ["light.kitchen"]}},
  {"entity_id": "scene.movie", "state": "unknown", "attributes": {"friendly_name": "Film"}},
  {"entity_id": "sensor.kitchen_temperature", "state": "21.5", "attributes": {"friendly_name": "Küche Temperatur", "device_class": "temperature", "unit_of_measurement": "°C"}},
  {"entity_id": "sensor.kitchen_battery", "state": "80", "attributes": {"device_class": "battery", "unit_of_measurement": "%"}},
  {"entity_id": "sensor.hall_humidity", "state": "unknown", "attributes": {"device_class": "humidity"}},
  {"entity_id": "binary_sensor.kitchen_window", "state": "on", "attributes": {"friendly_name": "Küchenfenster", "device_class": "window"}},
  {"entity_id": "binary_sensor.hall_motion", "state": "unknown", "attributes": {"device_class": "motion"}},
  {"entity_id": "media_player.tv", "state": "playing", "attributes": {"entity_id": "light.kitchen"}},
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
	mux.HandleFunc("GET /api/websocket", func(w http.ResponseWriter, r *http.Request) { f.websocket(t, w, r) })
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

// websocket answers the registry commands Layout sends, after the in-band sign-in.
func (f *fake) websocket(t *testing.T, w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		t.Error(err)
		return
	}
	defer conn.CloseNow() //nolint:errcheck // test server
	ctx := r.Context()
	_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_required"})
	var auth struct {
		AccessToken string `json:"access_token"`
	}
	if err := wsjson.Read(ctx, conn, &auth); err != nil {
		return
	}
	if auth.AccessToken != "access-"+strconv.Itoa(f.issued) {
		_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_invalid"})
		return
	}
	_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_ok"})
	results := map[string]string{
		"config/area_registry/list":   `[{"area_id": "living_room", "name": "Wohnzimmer"}, {"area_id": "kitchen", "name": "Küche"}]`,
		"config/device_registry/list": `[{"id": "d1", "area_id": "kitchen"}, {"id": "d2", "area_id": null}]`,
		"config/entity_registry/list_for_display": `{"entity_categories": {"0": "config", "1": "diagnostic"}, "entities": [
			{"ei": "light.kitchen", "di": "d1"},
			{"ei": "sensor.kitchen_temperature", "di": "d2", "ai": "kitchen"},
			{"ei": "sensor.kitchen_battery", "di": "d1", "ai": "", "ec": 1},
			{"ei": "light.living", "ai": "living_room"},
			{"ei": "light.sofa", "hb": true}
		]}`,
	}
	// Answer in reverse order, with an event in between, as Home Assistant may.
	type command struct {
		ID   int    `json:"id"`
		Type string `json:"type"`
	}
	var cmds []command
	for range results {
		var cmd command
		if err := wsjson.Read(ctx, conn, &cmd); err != nil {
			return
		}
		cmds = append(cmds, cmd)
	}
	_ = wsjson.Write(ctx, conn, map[string]any{"id": 99, "type": "event"})
	for _, cmd := range slices.Backward(cmds) {
		_ = wsjson.Write(ctx, conn, map[string]any{"id": cmd.ID, "type": "result", "success": true, "result": json.RawMessage(results[cmd.Type])})
	}
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

func TestDevicesAreScenesLightsAndReadings(t *testing.T) {
	c, refresh := signIn(t, &fake{})
	d, err := c.Devices(context.Background(), refresh)
	if err != nil {
		t.Fatal(err)
	}
	// The light group stands for its members; a scene's entities aren't a group.
	if !slices.Equal(d.Scenes, []domain.Scene{{ID: "scene.dinner", Name: "Abendessen"}, {ID: "scene.movie", Name: "Film"}}) ||
		!slices.Equal(d.Lights, []domain.Light{{ID: "light.kitchen", Name: "Küche", On: true}, {ID: "light.living", Name: "Wohnzimmer"}}) {
		t.Fatalf("got %v %v", d.Scenes, d.Lights)
	}
	if !slices.Equal(d.Readings, []domain.Reading{
		{ID: "sensor.kitchen_temperature", Name: "Küche Temperatur", Kind: domain.Temperature, Value: 21.5, Unit: "°C"},
		{ID: "binary_sensor.kitchen_window", Name: "Küchenfenster", Kind: domain.Window, On: true},
	}) {
		t.Fatalf("readings %v", d.Readings)
	}
}

func TestLayout(t *testing.T) {
	f := &fake{}
	c, refresh := signIn(t, f)
	l, err := c.Layout(context.Background(), refresh)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(l.Rooms, []domain.Room{{ID: "kitchen", Name: "Küche"}, {ID: "living_room", Name: "Wohnzimmer"}}) {
		t.Fatalf("rooms %v", l.Rooms)
	}
	// An entity's own area wins over its device's.
	if !maps.Equal(l.Areas, map[string]string{"light.kitchen": "kitchen", "sensor.kitchen_temperature": "kitchen", "sensor.kitchen_battery": "kitchen", "light.living": "living_room"}) {
		t.Fatalf("areas %v", l.Areas)
	}
	if !maps.Equal(l.Hidden, map[string]bool{"sensor.kitchen_battery": true, "light.sofa": true}) {
		t.Fatalf("hidden %v", l.Hidden)
	}

	// A stale access token is refused on the WebSocket too, and the next call gets a fresh one.
	f.issued++
	if _, err := c.Layout(context.Background(), refresh); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("stale access token: %v", err)
	}
	if _, err := c.Layout(context.Background(), refresh); err != nil {
		t.Fatalf("after refresh: %v", err)
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
	if _, err := c.Devices(ctx, refresh); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("stale access token: %v", err)
	}
	if _, err := c.Devices(ctx, refresh); err != nil {
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
	if _, err := c.Devices(ctx, refresh); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("revoked refresh token: %v", err)
	}
}

func TestUnavailable(t *testing.T) {
	f := &fake{}
	c, refresh := signIn(t, f)
	f.down = true
	if _, err := c.Devices(context.Background(), refresh); !errors.Is(err, domain.ErrUnavailable) {
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
