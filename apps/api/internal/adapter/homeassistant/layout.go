package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// registries are the lists Layout reads, in the order of their command ids (1, 2, 3). They are
// the ones Home Assistant's own dashboards read, so they work for people who aren't admins.
var registries = []string{"config/area_registry/list", "config/device_registry/list", "config/entity_registry/list_for_display"}

type area struct {
	ID   string `json:"area_id"`
	Name string `json:"name"`
}

type device struct {
	ID   string `json:"id"`
	Area string `json:"area_id"`
}

// entity is the compact form of list_for_display: ai area, di device, ec entity category (config
// or diagnostic), hb hidden.
type entity struct {
	ID       string `json:"ei"`
	Area     string `json:"ai"`
	Device   string `json:"di"`
	Category *int   `json:"ec"`
	Hidden   bool   `json:"hb"`
}

// message is anything Home Assistant sends on the WebSocket API.
type message struct {
	ID      int             `json:"id"`
	Type    string          `json:"type"`
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Layout reads the area, device and entity registries. Home Assistant only offers them over its
// WebSocket API, so this opens one connection, signs in and asks for all three.
func (c *Client) Layout(ctx context.Context, refreshToken string) (domain.Layout, error) {
	access, err := c.accessFor(ctx, refreshToken)
	if err != nil {
		return domain.Layout{}, err
	}
	results, err := c.ask(ctx, refreshToken, access)
	if err != nil {
		return domain.Layout{}, err
	}
	var (
		areas    []area
		devices  []device
		entities struct {
			Entities []entity `json:"entities"`
		}
	)
	for i, out := range []any{&areas, &devices, &entities} {
		if err := json.Unmarshal(results[i], out); err != nil {
			return domain.Layout{}, fmt.Errorf("%w: home assistant %s: %w", domain.ErrUnavailable, registries[i], err)
		}
	}

	l := domain.Layout{Rooms: make([]domain.Room, 0, len(areas)), Areas: map[string]string{}, Hidden: map[string]bool{}}
	for _, a := range areas {
		l.Rooms = append(l.Rooms, domain.Room{ID: a.ID, Name: a.Name})
	}
	sort.Slice(l.Rooms, func(i, j int) bool { return l.Rooms[i].Name < l.Rooms[j].Name })
	deviceArea := make(map[string]string, len(devices))
	for _, d := range devices {
		deviceArea[d.ID] = d.Area
	}
	for _, e := range entities.Entities {
		// An entity's own area wins over its device's.
		if a := e.Area; a != "" {
			l.Areas[e.ID] = a
		} else if a := deviceArea[e.Device]; a != "" {
			l.Areas[e.ID] = a
		}
		if e.Hidden || e.Category != nil {
			l.Hidden[e.ID] = true
		}
	}
	return l, nil
}

// ask signs in on the WebSocket API and returns the result of each of the registries.
func (c *Client) ask(ctx context.Context, refreshToken, access string) ([]json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.http.Timeout)
	defer cancel()
	u := c.base.JoinPath("/api/websocket")
	u.Scheme = "ws"
	if c.base.Scheme == "https" {
		u.Scheme = "wss"
	}
	conn, resp, err := websocket.Dial(ctx, u.String(), nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: home assistant websocket: %w", domain.ErrUnavailable, err)
	}
	defer conn.CloseNow() //nolint:errcheck // nothing left to say to it
	conn.SetReadLimit(16 << 20)

	var msg message
	fail := func(err error) ([]json.RawMessage, error) {
		return nil, fmt.Errorf("%w: home assistant websocket: %w", domain.ErrUnavailable, err)
	}
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		return fail(err)
	}
	if msg.Type != "auth_required" {
		return fail(fmt.Errorf("unexpected %q", msg.Type))
	}
	if err := wsjson.Write(ctx, conn, map[string]string{"type": "auth", "access_token": access}); err != nil {
		return fail(err)
	}
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		return fail(err)
	}
	if msg.Type != "auth_ok" {
		// Like a refused access token over REST: the next call gets a fresh one.
		c.forget(refreshToken)
		return fail(fmt.Errorf("sign-in refused: %q", msg.Type))
	}
	for i, cmd := range registries {
		if err := wsjson.Write(ctx, conn, map[string]any{"id": i + 1, "type": cmd}); err != nil {
			return fail(err)
		}
	}
	results := make([]json.RawMessage, len(registries))
	for got := 0; got < len(registries); {
		msg = message{}
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return fail(err)
		}
		if msg.Type != "result" || msg.ID < 1 || msg.ID > len(registries) || results[msg.ID-1] != nil {
			continue
		}
		if !msg.Success {
			return fail(fmt.Errorf("%s: %s %s", registries[msg.ID-1], msg.Error.Code, msg.Error.Message))
		}
		results[msg.ID-1] = msg.Result
		got++
	}
	return results, nil
}
