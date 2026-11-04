// Package openmeteo is the driven adapter for the day's weather. It asks Open-Meteo, which needs
// no key, for today's forecast at one place.
package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

const baseURL = "https://api.open-meteo.com"

type Client struct {
	base     string
	lat, lon float64
	http     *http.Client
}

var _ port.Forecaster = (*Client)(nil)

// New forecasts for the given place. Each call is bounded to 5 seconds.
func New(lat, lon float64) *Client {
	return &Client{base: baseURL, lat: lat, lon: lon, http: &http.Client{Timeout: 5 * time.Second}}
}

func (c *Client) Today(ctx context.Context) (domain.Weather, error) {
	q := url.Values{
		"latitude":      {strconv.FormatFloat(c.lat, 'f', 4, 64)},
		"longitude":     {strconv.FormatFloat(c.lon, 'f', 4, 64)},
		"daily":         {"weather_code,temperature_2m_max,temperature_2m_min"},
		"timezone":      {"auto"},
		"forecast_days": {"1"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/forecast?"+q.Encode(), nil)
	if err != nil {
		return domain.Weather{}, fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return domain.Weather{}, fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return domain.Weather{}, fmt.Errorf("%w: open-meteo answered %d", domain.ErrUnavailable, res.StatusCode)
	}
	var body struct {
		Daily struct {
			Code []int     `json:"weather_code"`
			Max  []float64 `json:"temperature_2m_max"`
			Min  []float64 `json:"temperature_2m_min"`
		} `json:"daily"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return domain.Weather{}, fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
	}
	d := body.Daily
	if len(d.Code) == 0 || len(d.Max) == 0 || len(d.Min) == 0 {
		return domain.Weather{}, fmt.Errorf("%w: no forecast", domain.ErrUnavailable)
	}
	return domain.Weather{Code: d.Code[0], High: d.Max[0], Low: d.Min[0]}, nil
}
