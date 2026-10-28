package plex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// fakePlex is plex.tv and two servers: Alice's own, and a friend's shared with her that is down.
func fakePlex(t *testing.T) (*Client, *int) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "server-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/identity":
		case "/library/sections":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"1","title":"Movies","type":"movie"},{"key":"2","title":"Series","type":"show"},{"key":"3","title":"Music","type":"artist"}]}}`))
		case "/library/sections/1/all":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
				{"ratingKey":"10","type":"movie","title":"Arrival","year":2016,"duration":6960000,"audienceRating":8.2,"viewCount":1,"Genre":[{"tag":"Sci-Fi"}]},
				{"ratingKey":"11","type":"movie","title":"Paddington","year":2014,"rating":7.5}]}}`))
		case "/library/sections/2/all":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"20","type":"show","title":"Dark","leafCount":26,"viewedLeafCount":26}]}}`))
		case "/photo/:/transcode":
			if r.URL.Query().Get("url") != "/library/metadata/10/thumb" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	resourceCalls := 0
	account := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Client-Identifier") == "" {
			t.Error("no client identifier")
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/pins":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":42,"code":"abcd"}`))
		case r.URL.Path == "/api/v2/pins/42":
			_, _ = w.Write([]byte(`{"id":42,"authToken":"alice-token"}`))
		case r.URL.Path == "/api/v2/resources":
			resourceCalls++
			if r.Header.Get("X-Plex-Token") != "alice-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode([]resource{
				{Name: "Friend", ClientIdentifier: "f00", Provides: "server", AccessToken: "other", Connections: []connection{{URI: "http://127.0.0.1:1"}}},
				{Name: "Home", ClientIdentifier: "abc", Provides: "server", Owned: true, AccessToken: "server-token", Connections: []connection{
					{URI: "http://127.0.0.1:1", Local: true}, {URI: server.URL},
				}},
				{Name: "iPhone", ClientIdentifier: "phone", Provides: "client,player"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(account.Close)

	c := New("https://homeloom.test/")
	c.account, c.clients = account.URL, account.URL
	return c, &resourceCalls
}

func TestSignIn(t *testing.T) {
	ctx := context.Background()
	c, _ := fakePlex(t)
	pin, to, err := c.StartSignIn(ctx, "s1")
	if err != nil || pin != "42" {
		t.Fatalf("start: %q, %v", pin, err)
	}
	u, _ := url.Parse(to)
	q, _ := url.ParseQuery(strings.TrimPrefix(u.Fragment, "?"))
	if u.Host != "app.plex.tv" || q.Get("code") != "abcd" || q.Get("clientID") != c.clientID || q.Get("forwardUrl") != "https://homeloom.test/auth/plex/callback?state=s1" {
		t.Fatalf("sign-in page: %s", to)
	}
	if token, err := c.FinishSignIn(ctx, pin); err != nil || token != "alice-token" {
		t.Fatalf("finish: %q, %v", token, err)
	}
	if _, err := c.FinishSignIn(ctx, "../resources"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("bad pin: %v", err)
	}
}

func TestLibrariesTitlesAndPosters(t *testing.T) {
	ctx := context.Background()
	c, resourceCalls := fakePlex(t)

	libs, err := c.Libraries(ctx, "alice-token")
	if err != nil {
		t.Fatal(err)
	}
	// Music is left out, and the friend's server, which is down, doesn't take the others with it.
	want := []domain.Library{
		{ID: "abc.1", Name: "Movies", Kind: domain.Movies, Server: "Home", Owned: true},
		{ID: "abc.2", Name: "Series", Kind: domain.Shows, Server: "Home", Owned: true},
	}
	if len(libs) != 2 || libs[0] != want[0] || libs[1] != want[1] {
		t.Fatalf("libraries: %+v", libs)
	}

	movies, err := c.Titles(ctx, "alice-token", "abc.1")
	if err != nil || len(movies) != 2 {
		t.Fatalf("movies: %+v, %v", movies, err)
	}
	if a := movies[0]; a.ID != "abc.10" || a.Minutes != 116 || a.Rating != 8.2 || !a.Watched || a.Genres[0] != "Sci-Fi" {
		t.Fatalf("arrival: %+v", a)
	}
	if p := movies[1]; p.Watched || p.Rating != 7.5 {
		t.Fatalf("paddington: %+v", p)
	}
	shows, err := c.Titles(ctx, "alice-token", "abc.2")
	if err != nil || len(shows) != 1 || !shows[0].Watched || shows[0].Episodes != 26 {
		t.Fatalf("shows: %+v, %v", shows, err)
	}

	p, err := c.Poster(ctx, "alice-token", "abc.10")
	if err != nil || p.ContentType != "image/jpeg" || string(p.Body) != "jpeg" {
		t.Fatalf("poster: %+v, %v", p, err)
	}
	if *resourceCalls != 1 {
		t.Fatalf("servers listed %d times, want once", *resourceCalls)
	}

	// Ids that aren't on one of the person's servers, or that try to reach elsewhere, are not found.
	for _, id := range []string{"zzz.1", "abc.1/../../identity", "abc", "abc.99"} {
		if _, err := c.Titles(ctx, "alice-token", id); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("titles %q: %v", id, err)
		}
	}
	if _, err := c.Libraries(ctx, "revoked"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("revoked token: %v", err)
	}
}
