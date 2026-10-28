// Package homeassistant is the driven adapter for Home Assistant's REST API. It only ever reads
// states and calls scene.turn_on, light.turn_on and light.turn_off.
package homeassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

var _ port.SmartHome = (*Client)(nil)

// New talks to the Home Assistant at baseURL with a long-lived access token.
func New(baseURL, token string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("HOME_ASSISTANT_URL %q is not an http(s) URL", baseURL)
	}
	return &Client{base: u, token: token, http: &http.Client{Timeout: 5 * time.Second}}, nil
}

type state struct {
	EntityID   string `json:"entity_id"`
	State      string `json:"state"`
	Attributes struct {
		FriendlyName string `json:"friendly_name"`
	} `json:"attributes"`
}

func (s state) name() string {
	if s.Attributes.FriendlyName != "" {
		return s.Attributes.FriendlyName
	}
	return s.EntityID
}

func (c *Client) Devices(ctx context.Context) ([]domain.Scene, []domain.Light, error) {
	var states []state
	if err := c.do(ctx, http.MethodGet, "/api/states", nil, &states); err != nil {
		return nil, nil, err
	}
	scenes, lights := []domain.Scene{}, []domain.Light{}
	for _, s := range states {
		if s.State == "unavailable" {
			continue
		}
		switch {
		case strings.HasPrefix(s.EntityID, "scene."):
			scenes = append(scenes, domain.Scene{ID: s.EntityID, Name: s.name()})
		case strings.HasPrefix(s.EntityID, "light."):
			lights = append(lights, domain.Light{ID: s.EntityID, Name: s.name(), On: s.State == "on"})
		}
	}
	sort.Slice(scenes, func(i, j int) bool { return scenes[i].Name < scenes[j].Name })
	sort.Slice(lights, func(i, j int) bool { return lights[i].Name < lights[j].Name })
	return scenes, lights, nil
}

func (c *Client) ActivateScene(ctx context.Context, id string) error {
	if !strings.HasPrefix(id, "scene.") {
		return fmt.Errorf("%w: %s is not a scene", domain.ErrNotFound, id)
	}
	return c.do(ctx, http.MethodPost, "/api/services/scene/turn_on", map[string]string{"entity_id": id}, nil)
}

// ToggleLight reads the light and asks for the opposite state, so the answer doesn't depend on
// how fast the light reports back.
func (c *Client) ToggleLight(ctx context.Context, id string) (domain.Light, error) {
	if !strings.HasPrefix(id, "light.") {
		return domain.Light{}, fmt.Errorf("%w: %s is not a light", domain.ErrNotFound, id)
	}
	var s state
	if err := c.do(ctx, http.MethodGet, "/api/states/"+url.PathEscape(id), nil, &s); err != nil {
		return domain.Light{}, err
	}
	service := "turn_on"
	if s.State == "on" {
		service = "turn_off"
	}
	if err := c.do(ctx, http.MethodPost, "/api/services/light/"+service, map[string]string{"entity_id": id}, nil); err != nil {
		return domain.Light{}, err
	}
	return domain.Light{ID: id, Name: s.name(), On: service == "turn_on"}, nil
}

// do sends one request. Home Assistant being down, slow or refusing the token is
// domain.ErrUnavailable; a 404 is domain.ErrNotFound.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base.JoinPath(path).String(), rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: home assistant %s %s: %w", domain.ErrUnavailable, method, path, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: home assistant %s", domain.ErrNotFound, path)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode >= 500:
		return fmt.Errorf("%w: home assistant %s %s: %s", domain.ErrUnavailable, method, path, resp.Status)
	case resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("home assistant %s %s: %s: %s", method, path, resp.Status, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out); err != nil {
		return fmt.Errorf("%w: home assistant %s: %w", domain.ErrUnavailable, path, err)
	}
	return nil
}
