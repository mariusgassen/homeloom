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

func TestSearchAndLabelledForecast(t *testing.T) {
	var asked []string
	geo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"results":[
			{"name":"Springfield","admin1":"Missouri","country":"United States","latitude":37.2,"longitude":-93.3},
			{"name":"Springfield","admin1":"Illinois","country":"United States","latitude":39.8,"longitude":-89.6},
			{"name":"Berlin","admin1":"Berlin","country":"Germany","latitude":52.5,"longitude":13.4},
			{"name":"Berlin","admin1":"Berlin","country":"Germany","latitude":1,"longitude":1}]}`))
	}))
	defer geo.Close()
	var forecastQuery string
	fc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forecastQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"daily":{"weather_code":[0],"temperature_2m_max":[20],"temperature_2m_min":[10],"precipitation_probability_max":[0]}}`))
	}))
	defer fc.Close()
	c := openmeteo.NewAt(geo.URL, fc.URL)

	got, err := c.Search(context.Background(), "Spring", "de", 5)
	want := []domain.PlaceMatch{
		{Label: "Springfield, Missouri, United States"}, {Label: "Springfield, Illinois, United States"}, {Label: "Berlin, Germany"},
	}
	if err != nil || len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("search: %+v, %v", got, err)
	}
	if !strings.Contains(asked[0], "language=de") || !strings.Contains(asked[0], "name=Spring") {
		t.Errorf("query: %s", asked[0])
	}

	// A label from the search finds the same place again, not the first one with that name.
	if _, err := c.Forecast(context.Background(), "Springfield, Illinois, United States", time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(forecastQuery, "latitude=39.8") || !strings.Contains(asked[1], "name=Springfield") {
		t.Errorf("forecast for %q, geocoded with %q", forecastQuery, asked[1])
	}
}
