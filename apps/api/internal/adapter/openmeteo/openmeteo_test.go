package openmeteo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func TestToday(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("latitude") != "52.5200" {
			t.Errorf("query %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"daily":{"weather_code":[61],"temperature_2m_max":[14.2],"temperature_2m_min":[7.9]}}`))
	}))
	defer srv.Close()
	c := New(52.52, 13.405)
	c.base = srv.URL
	w, err := c.Today(context.Background())
	if err != nil || w != (domain.Weather{Code: 61, High: 14.2, Low: 7.9}) {
		t.Fatalf("got %+v, %v", w, err)
	}
}

func TestTodayUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	c := New(1, 2)
	c.base = srv.URL
	if _, err := c.Today(context.Background()); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}
