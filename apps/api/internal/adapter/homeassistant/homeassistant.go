// Package homeassistant is the driven adapter for Home Assistant's REST API. It signs in through
// Home Assistant's own OAuth flow and only ever reads states and the area, device and entity
// registries, and calls scene.turn_on, light.turn_on and light.turn_off.
package homeassistant

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// CallbackPath is where Home Assistant sends the browser back after signing in.
const CallbackPath = "/auth/homeassistant/callback"

// Client acts for whoever signed in. Home Assistant needs no client registration: the client id
// is the app's URL, and the redirect URI must be on the same host.
type Client struct {
	base        *url.URL
	clientID    string
	redirectURI string
	http        *http.Client

	mu     sync.Mutex
	access map[[32]byte]accessToken // by hash of the refresh token
}

type accessToken struct {
	token   string
	expires time.Time
}

var _ port.SmartHome = (*Client)(nil)

// New talks to the Home Assistant at baseURL for the app served at publicURL.
func New(baseURL, publicURL string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("HOME_ASSISTANT_URL %q is not an http(s) URL", baseURL)
	}
	app := strings.TrimRight(publicURL, "/")
	return &Client{
		base: u, clientID: app + "/", redirectURI: app + CallbackPath,
		http:   &http.Client{Timeout: 5 * time.Second},
		access: map[[32]byte]accessToken{},
	}, nil
}

func (c *Client) AuthorizeURL(state string) string {
	u := c.base.JoinPath("/auth/authorize")
	u.RawQuery = url.Values{"response_type": {"code"}, "client_id": {c.clientID}, "redirect_uri": {c.redirectURI}, "state": {state}}.Encode()
	return u.String()
}

type tokens struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func (c *Client) Exchange(ctx context.Context, code string) (string, error) {
	t, err := c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}})
	if err != nil {
		return "", err
	}
	if t.RefreshToken == "" {
		return "", fmt.Errorf("%w: home assistant sent no refresh token", domain.ErrUnavailable)
	}
	c.remember(t.RefreshToken, t)
	return t.RefreshToken, nil
}

func (c *Client) Revoke(ctx context.Context, refreshToken string) error {
	c.forget(refreshToken)
	req, err := c.form(ctx, "/auth/revoke", url.Values{"token": {refreshToken}})
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: home assistant revoke: %w", domain.ErrUnavailable, err)
	}
	return resp.Body.Close()
}

// accessFor returns a current access token, refreshing it shortly before it runs out.
func (c *Client) accessFor(ctx context.Context, refreshToken string) (string, error) {
	key := sha256.Sum256([]byte(refreshToken))
	c.mu.Lock()
	a, ok := c.access[key]
	c.mu.Unlock()
	if ok && time.Until(a.expires) > time.Minute {
		return a.token, nil
	}
	t, err := c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}})
	if err != nil {
		return "", err
	}
	c.remember(refreshToken, t)
	return t.AccessToken, nil
}

func (c *Client) remember(refreshToken string, t tokens) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.access[sha256.Sum256([]byte(refreshToken))] = accessToken{token: t.AccessToken, expires: time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)}
}

func (c *Client) forget(refreshToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.access, sha256.Sum256([]byte(refreshToken)))
}

// token calls the token endpoint. A rejected code or refresh token is domain.ErrUnauthenticated.
func (c *Client) token(ctx context.Context, form url.Values) (tokens, error) {
	form.Set("client_id", c.clientID)
	req, err := c.form(ctx, "/auth/token", form)
	if err != nil {
		return tokens{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return tokens{}, fmt.Errorf("%w: home assistant token: %w", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return tokens{}, fmt.Errorf("%w: home assistant token: %s", domain.ErrUnauthenticated, resp.Status)
	case resp.StatusCode >= 300:
		return tokens{}, fmt.Errorf("%w: home assistant token: %s", domain.ErrUnavailable, resp.Status)
	}
	var t tokens
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&t); err != nil || t.AccessToken == "" {
		return tokens{}, fmt.Errorf("%w: home assistant token: unreadable answer", domain.ErrUnavailable)
	}
	return t, nil
}

func (c *Client) form(ctx context.Context, path string, form url.Values) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base.JoinPath(path).String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, nil
}

type state struct {
	EntityID   string `json:"entity_id"`
	State      string `json:"state"`
	Attributes struct {
		FriendlyName string `json:"friendly_name"`
		DeviceClass  string `json:"device_class"`
		Unit         string `json:"unit_of_measurement"`
		// Members of a group helper; a scene lists the entities it sets here too.
		EntityID members `json:"entity_id"`
	} `json:"attributes"`
}

// members is a group's entity_id attribute, which some entities use for a single id instead.
type members []string

func (m *members) UnmarshalJSON(b []byte) error {
	var ids []string
	if json.Unmarshal(b, &ids) == nil {
		*m = ids
	}
	return nil
}

func (s state) name() string {
	if s.Attributes.FriendlyName != "" {
		return s.Attributes.FriendlyName
	}
	return s.EntityID
}

// measured and detected are the sensor and binary sensor device classes shown as readings.
var (
	measured = map[string]domain.ReadingKind{
		"temperature": domain.Temperature, "humidity": domain.Humidity, "power": domain.Power, "carbon_dioxide": domain.CO2,
	}
	detected = map[string]domain.ReadingKind{
		"window": domain.Window, "door": domain.Door, "garage_door": domain.GarageDoor,
		"motion": domain.Motion, "occupancy": domain.Motion, "moisture": domain.Moisture, "smoke": domain.Smoke,
	}
)

func (c *Client) Devices(ctx context.Context, refreshToken string) (domain.Devices, error) {
	var states []state
	if err := c.do(ctx, refreshToken, http.MethodGet, "/api/states", nil, &states); err != nil {
		return domain.Devices{}, err
	}
	// A group helper stands for its members, so they are left out. Scenes list entities too,
	// but don't group them.
	grouped := map[string]bool{}
	for _, s := range states {
		if !strings.HasPrefix(s.EntityID, "scene.") {
			for _, id := range s.Attributes.EntityID {
				grouped[id] = true
			}
		}
	}
	d := domain.Devices{Scenes: []domain.Scene{}, Lights: []domain.Light{}, Readings: []domain.Reading{}}
	for _, s := range states {
		if s.State == "unavailable" || grouped[s.EntityID] {
			continue
		}
		domainName, _, _ := strings.Cut(s.EntityID, ".")
		switch domainName {
		case "scene":
			d.Scenes = append(d.Scenes, domain.Scene{ID: s.EntityID, Name: s.name()})
		case "light":
			d.Lights = append(d.Lights, domain.Light{ID: s.EntityID, Name: s.name(), On: s.State == "on"})
		case "sensor":
			kind, ok := measured[s.Attributes.DeviceClass]
			v, err := strconv.ParseFloat(s.State, 64)
			if ok && err == nil {
				d.Readings = append(d.Readings, domain.Reading{ID: s.EntityID, Name: s.name(), Kind: kind, Value: v, Unit: s.Attributes.Unit})
			}
		case "binary_sensor":
			if kind, ok := detected[s.Attributes.DeviceClass]; ok && (s.State == "on" || s.State == "off") {
				d.Readings = append(d.Readings, domain.Reading{ID: s.EntityID, Name: s.name(), Kind: kind, On: s.State == "on"})
			}
		}
	}
	sort.Slice(d.Scenes, func(i, j int) bool { return d.Scenes[i].Name < d.Scenes[j].Name })
	sort.Slice(d.Lights, func(i, j int) bool { return d.Lights[i].Name < d.Lights[j].Name })
	sort.Slice(d.Readings, func(i, j int) bool { return d.Readings[i].Name < d.Readings[j].Name })
	return d, nil
}

func (c *Client) ActivateScene(ctx context.Context, refreshToken, id string) error {
	if !strings.HasPrefix(id, "scene.") {
		return fmt.Errorf("%w: %s is not a scene", domain.ErrNotFound, id)
	}
	return c.do(ctx, refreshToken, http.MethodPost, "/api/services/scene/turn_on", map[string]string{"entity_id": id}, nil)
}

// ToggleLight reads the light and asks for the opposite state, so the answer doesn't depend on
// how fast the light reports back.
func (c *Client) ToggleLight(ctx context.Context, refreshToken, id string) (domain.Light, error) {
	if !strings.HasPrefix(id, "light.") {
		return domain.Light{}, fmt.Errorf("%w: %s is not a light", domain.ErrNotFound, id)
	}
	var s state
	if err := c.do(ctx, refreshToken, http.MethodGet, "/api/states/"+url.PathEscape(id), nil, &s); err != nil {
		return domain.Light{}, err
	}
	service := "turn_on"
	if s.State == "on" {
		service = "turn_off"
	}
	if err := c.do(ctx, refreshToken, http.MethodPost, "/api/services/light/"+service, map[string]string{"entity_id": id}, nil); err != nil {
		return domain.Light{}, err
	}
	return domain.Light{ID: id, Name: s.name(), On: service == "turn_on"}, nil
}

// do sends one request as the person behind refreshToken. Home Assistant being down or slow is
// domain.ErrUnavailable, and so is a refused access token (the next call gets a fresh one); a
// 404 is domain.ErrNotFound.
func (c *Client) do(ctx context.Context, refreshToken, method, path string, body, out any) error {
	access, err := c.accessFor(ctx, refreshToken)
	if err != nil {
		return err
	}
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
	req.Header.Set("Authorization", "Bearer "+access)
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
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		c.forget(refreshToken)
		return fmt.Errorf("%w: home assistant %s %s: %s", domain.ErrUnavailable, method, path, resp.Status)
	case resp.StatusCode >= 500:
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
