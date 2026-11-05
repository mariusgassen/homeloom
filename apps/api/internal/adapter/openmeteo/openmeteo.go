// Package openmeteo forecasts the weather with Open-Meteo, which is free and needs no key.
package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

const (
	geocodingURL = "https://geocoding-api.open-meteo.com"
	forecastURL  = "https://api.open-meteo.com"
	timeout      = 5 * time.Second
	maxBody      = 1 << 20
)

// Client asks Open-Meteo for forecasts.
type Client struct {
	http                 *http.Client
	geocoding, forecasts string
}

var (
	_ port.Weather     = (*Client)(nil)
	_ port.PlaceSearch = (*Client)(nil)
)

func New() *Client {
	return &Client{http: &http.Client{Timeout: timeout}, geocoding: geocodingURL, forecasts: forecastURL}
}

// NewAt points the client at other hosts, for tests.
func NewAt(geocoding, forecasts string) *Client {
	return &Client{http: &http.Client{Timeout: timeout}, geocoding: geocoding, forecasts: forecasts}
}

// place is a geocoding result.
type place struct {
	Name      string  `json:"name"`
	Admin1    string  `json:"admin1"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// label is how the place is written: "Berlin, Germany", or "Springfield, Illinois, United States".
func (p place) label() string {
	var parts []string
	for _, v := range []string{p.Name, p.Admin1, p.Country} {
		if v != "" && (len(parts) == 0 || !strings.EqualFold(parts[len(parts)-1], v)) {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ", ")
}

// find returns the places whose name starts like the text before its first comma, most populous
// first. The geocoder only searches names, so "Springfield, Illinois" is told apart afterwards.
func (c *Client) find(ctx context.Context, text, lang string, count int) ([]place, error) {
	name, _, _ := strings.Cut(text, ",")
	q := url.Values{"name": {strings.TrimSpace(name)}, "count": {fmt.Sprint(count)}, "format": {"json"}}
	if lang != "" {
		q.Set("language", lang)
	}
	var geo struct {
		Results []place `json:"results"`
	}
	if err := c.get(ctx, c.geocoding+"/v1/search", q, &geo); err != nil {
		return nil, err
	}
	return geo.Results, nil
}

// Search suggests places for text typed so far. The geocoder matches whole names, so a name that
// has only just begun may find nothing.
func (c *Client) Search(ctx context.Context, text, lang string, n int) ([]domain.PlaceMatch, error) {
	found, err := c.find(ctx, text, lang, n+5)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PlaceMatch, 0, n)
	seen := map[string]bool{}
	for _, p := range found {
		if l := p.label(); l != "" && !seen[strings.ToLower(l)] && len(out) < n {
			seen[strings.ToLower(l)] = true
			out = append(out, domain.PlaceMatch{Label: l})
		}
	}
	return out, nil
}

func (c *Client) Forecast(ctx context.Context, text string, day time.Time) (domain.Forecast, error) {
	found, err := c.find(ctx, text, "", 10)
	if err != nil {
		return domain.Forecast{}, err
	}
	if len(found) == 0 {
		return domain.Forecast{}, domain.ErrNotFound
	}
	chosen := found[0]
	for _, p := range found {
		if strings.EqualFold(p.label(), strings.TrimSpace(text)) {
			chosen = p
			break
		}
	}
	d := day.Format(time.DateOnly)
	var f struct {
		Daily struct {
			Code []int      `json:"weather_code"`
			High []*float64 `json:"temperature_2m_max"`
			Low  []*float64 `json:"temperature_2m_min"`
			Rain []*int     `json:"precipitation_probability_max"`
		} `json:"daily"`
	}
	err = c.get(ctx, c.forecasts+"/v1/forecast", url.Values{
		"latitude":  {fmt.Sprint(chosen.Latitude)},
		"longitude": {fmt.Sprint(chosen.Longitude)},
		"daily":     {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max"},
		"timezone":  {"auto"}, "start_date": {d}, "end_date": {d},
	}, &f)
	if err != nil {
		return domain.Forecast{}, err
	}
	if len(f.Daily.Code) == 0 || len(f.Daily.High) == 0 || len(f.Daily.Low) == 0 || f.Daily.High[0] == nil || f.Daily.Low[0] == nil {
		return domain.Forecast{}, fmt.Errorf("%w: open-meteo gave no forecast for %s", domain.ErrUnavailable, d)
	}
	out := domain.Forecast{Sky: sky(f.Daily.Code[0]), HighC: *f.Daily.High[0], LowC: *f.Daily.Low[0]}
	if len(f.Daily.Rain) > 0 && f.Daily.Rain[0] != nil {
		out.RainPercent = *f.Daily.Rain[0]
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, endpoint string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: open-meteo: %w", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: open-meteo answered %d", domain.ErrUnavailable, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return fmt.Errorf("%w: open-meteo answered no JSON: %w", domain.ErrUnavailable, err)
	}
	return nil
}

// sky maps a WMO weather code to a word.
func sky(code int) domain.Sky {
	switch {
	case code <= 1:
		return domain.SkyClear
	case code <= 3:
		return domain.SkyCloudy
	case code == 45 || code == 48:
		return domain.SkyFog
	case code >= 95:
		return domain.SkyStorm
	case code >= 71 && code <= 77 || code == 85 || code == 86:
		return domain.SkySnow
	default: // drizzle, rain, showers
		return domain.SkyRain
	}
}
