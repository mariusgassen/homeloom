package openmeteo_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/openmeteo"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func TestForecast(t *testing.T) {
	geo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "Nowhere" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"latitude":52.5,"longitude":13.4}]}`))
	}))
	defer geo.Close()
	var query string
	fc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"daily":{"weather_code":[61],"temperature_2m_max":[11.5],"temperature_2m_min":[4],"precipitation_probability_max":[70]}}`))
	}))
	defer fc.Close()
	c := openmeteo.NewAt(geo.URL, fc.URL)

	day := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	got, err := c.Forecast(context.Background(), "Berlin", day)
	if err != nil || got != (domain.Forecast{Sky: domain.SkyRain, HighC: 11.5, LowC: 4, RainPercent: 70}) {
		t.Fatalf("forecast: %+v, %v", got, err)
	}
	if want := "2026-11-07"; !contains(query, "start_date="+want) || !contains(query, "end_date="+want) {
		t.Errorf("query: %s", query)
	}
	if _, err := c.Forecast(context.Background(), "Nowhere", day); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown place: want ErrNotFound, got %v", err)
	}
	fc.Close()
	if _, err := c.Forecast(context.Background(), "Berlin", day); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("down: want ErrUnavailable, got %v", err)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
