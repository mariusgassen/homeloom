package bigdatacloud_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/bigdatacloud"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func TestPlaceName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("latitude") != "52.52" || q.Get("longitude") != "13.4" || q.Get("localityLanguage") != "de" {
			t.Errorf("query %v", q)
		}
		_, _ = w.Write([]byte(`{"city":"","locality":"Mitte"}`))
	}))
	defer srv.Close()
	got, err := bigdatacloud.NewAt(srv.URL).PlaceName(context.Background(), domain.Coords{Latitude: 52.52, Longitude: 13.4}, "de-DE")
	if err != nil || got != "Mitte" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestPlaceNameUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer srv.Close()
	_, err := bigdatacloud.NewAt(srv.URL).PlaceName(context.Background(), domain.Coords{}, "")
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("err %v", err)
	}
}
