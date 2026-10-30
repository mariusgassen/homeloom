// Package plex is the driven adapter for Plex. It signs in with Plex's PIN flow, finds the
// person's servers (their own and those friends share) at plex.tv, and only ever reads movie and
// series libraries, their titles and posters from those servers.
package plex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// CallbackPath is where Plex sends the browser back after signing in.
const CallbackPath = "/auth/plex/callback"

const (
	product = "Homeloom"
	// cacheFor is how long the server list and the connection picked for a server are reused, so
	// a screen of posters doesn't ask plex.tv for every one.
	cacheFor = 5 * time.Minute
	// probeTimeout bounds trying one address of a server; a server has several (local, remote, relay).
	probeTimeout = 3 * time.Second
	maxPoster    = 5 << 20
)

// Client acts for whoever signed in. Plex needs no app registration; clientID names this
// Homeloom to Plex, so people see it under their authorized devices.
type Client struct {
	account  string // plex.tv
	clients  string // clients.plex.tv, which lists servers
	clientID string
	callback string
	http     *http.Client

	mu      sync.Mutex
	servers map[[32]byte]cachedServers // by hash of the person's token
}

type cachedServers struct {
	servers []*server
	expires time.Time
}

// server is one Plex server the person can reach. base is found on first use.
type server struct {
	id, name    string
	owned       bool
	token       string
	connections []connection

	mu   sync.Mutex
	base string
}

var _ port.MediaLibrary = (*Client)(nil)

// New signs in for the app served at publicURL.
func New(publicURL string) *Client {
	app := strings.TrimRight(publicURL, "/")
	id := sha256.Sum256([]byte("homeloom:" + app))
	return &Client{
		account: "https://plex.tv", clients: "https://clients.plex.tv",
		clientID: hex.EncodeToString(id[:16]), callback: app + CallbackPath,
		http:    &http.Client{Timeout: 10 * time.Second},
		servers: map[[32]byte]cachedServers{},
	}
}

type pin struct {
	ID        int    `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
}

func (c *Client) StartSignIn(ctx context.Context, state string) (string, string, error) {
	var p pin
	if err := c.call(ctx, http.MethodPost, c.account+"/api/v2/pins?strong=true", "", &p); err != nil {
		return "", "", err
	}
	if p.ID == 0 || p.Code == "" {
		return "", "", fmt.Errorf("%w: plex sent no pin", domain.ErrUnavailable)
	}
	q := url.Values{
		"clientID":                 {c.clientID},
		"code":                     {p.Code},
		"forwardUrl":               {c.callback + "?" + url.Values{"state": {state}}.Encode()},
		"context[device][product]": {product},
	}
	// Plex's sign-in page reads its parameters from the fragment.
	return strconv.Itoa(p.ID), "https://app.plex.tv/auth#?" + q.Encode(), nil
}

func (c *Client) FinishSignIn(ctx context.Context, pinID string) (string, error) {
	if _, err := strconv.Atoi(pinID); err != nil {
		return "", fmt.Errorf("%w: bad pin", domain.ErrUnauthenticated)
	}
	// Plex can be a moment behind its own redirect, so look a few times.
	for try := range 3 {
		if try > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Second):
			}
		}
		var p pin
		err := c.call(ctx, http.MethodGet, c.account+"/api/v2/pins/"+pinID, "", &p)
		if err != nil {
			return "", err
		}
		if p.AuthToken != "" {
			return p.AuthToken, nil
		}
	}
	return "", fmt.Errorf("%w: plex sign-in not approved", domain.ErrUnauthenticated)
}

func (c *Client) Libraries(ctx context.Context, token string) ([]domain.Library, error) {
	servers, err := c.serversFor(ctx, token)
	if err != nil {
		return nil, err
	}
	found := make([][]domain.Library, len(servers))
	failed := make([]error, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Go(func() { found[i], failed[i] = c.sections(ctx, s) })
	}
	wg.Wait()
	out := []domain.Library{}
	reached := false
	for i, s := range servers {
		if failed[i] != nil {
			// One server being off leaves the others.
			slog.WarnContext(ctx, "plex server unavailable", "server", s.name, "err", failed[i])
			continue
		}
		reached = true
		out = append(out, found[i]...)
	}
	if !reached && len(servers) > 0 {
		return nil, fmt.Errorf("%w: no plex server reachable", domain.ErrUnavailable)
	}
	return out, nil
}

type container struct {
	MediaContainer struct {
		Directory []struct {
			Key   string `json:"key"`
			Title string `json:"title"`
			Type  string `json:"type"`
		} `json:"Directory"`
		Metadata []metadata `json:"Metadata"`
	} `json:"MediaContainer"`
}

type metadata struct {
	RatingKey       string  `json:"ratingKey"`
	Type            string  `json:"type"`
	Title           string  `json:"title"`
	Year            int     `json:"year"`
	Summary         string  `json:"summary"`
	Duration        int     `json:"duration"` // milliseconds
	ContentRating   string  `json:"contentRating"`
	AudienceRating  float64 `json:"audienceRating"`
	Rating          float64 `json:"rating"`
	ViewCount       int     `json:"viewCount"`
	LeafCount       int     `json:"leafCount"`
	ViewedLeafCount int     `json:"viewedLeafCount"`
	Genre           []struct {
		Tag string `json:"tag"`
	} `json:"Genre"`
}

func (c *Client) sections(ctx context.Context, s *server) ([]domain.Library, error) {
	var body container
	if err := c.fromServer(ctx, s, "/library/sections", &body); err != nil {
		return nil, err
	}
	var out []domain.Library
	for _, d := range body.MediaContainer.Directory {
		kind := domain.MediaKind(d.Type)
		if kind != domain.Movies && kind != domain.Shows {
			continue
		}
		out = append(out, domain.Library{ID: s.id + "." + d.Key, Name: d.Title, Kind: kind, Server: s.name, Owned: s.owned})
	}
	return out, nil
}

func (c *Client) Titles(ctx context.Context, token, libraryID string) ([]domain.Title, error) {
	s, key, err := c.find(ctx, token, libraryID)
	if err != nil {
		return nil, err
	}
	var body container
	if err := c.fromServer(ctx, s, "/library/sections/"+key+"/all", &body); err != nil {
		return nil, err
	}
	out := []domain.Title{}
	for _, m := range body.MediaContainer.Metadata {
		if t, ok := toTitle(s, m); ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func (c *Client) Title(ctx context.Context, token, titleID string) (domain.Title, error) {
	s, key, err := c.find(ctx, token, titleID)
	if err != nil {
		return domain.Title{}, err
	}
	var body container
	if err := c.fromServer(ctx, s, "/library/metadata/"+key, &body); err != nil {
		return domain.Title{}, err
	}
	for _, m := range body.MediaContainer.Metadata {
		if t, ok := toTitle(s, m); ok && m.RatingKey == key {
			return t, nil
		}
	}
	return domain.Title{}, domain.ErrNotFound
}

// toTitle reads a movie or series; anything else is not a title.
func toTitle(s *server, m metadata) (domain.Title, bool) {
	kind := domain.MediaKind(m.Type)
	if kind != domain.Movies && kind != domain.Shows {
		return domain.Title{}, false
	}
	t := domain.Title{
		ID: s.id + "." + m.RatingKey, Kind: kind, Name: m.Title, Year: m.Year, Summary: m.Summary,
		Minutes: m.Duration / 60000, ContentRating: m.ContentRating, Rating: m.AudienceRating, Genres: []string{},
		// Plex's own app opens the title on that server, and plays it from there.
		URL: "https://app.plex.tv/desktop/#!/server/" + s.id + "/details?key=" + url.QueryEscape("/library/metadata/"+m.RatingKey),
	}
	if t.Rating == 0 {
		t.Rating = m.Rating
	}
	for _, g := range m.Genre {
		t.Genres = append(t.Genres, g.Tag)
	}
	if kind == domain.Movies {
		t.Watched = m.ViewCount > 0
	} else {
		t.Episodes = m.LeafCount
		t.Watched = m.LeafCount > 0 && m.ViewedLeafCount >= m.LeafCount
	}
	return t, true
}

func (c *Client) Poster(ctx context.Context, token, titleID string) (domain.Poster, error) {
	s, key, err := c.find(ctx, token, titleID)
	if err != nil {
		return domain.Poster{}, err
	}
	q := url.Values{"width": {"342"}, "height": {"513"}, "minSize": {"1"}, "upscale": {"1"}, "url": {"/library/metadata/" + key + "/thumb"}}
	resp, err := c.serverGet(ctx, s, "/photo/:/transcode?"+q.Encode(), "image/*")
	if err != nil {
		return domain.Poster{}, err
	}
	defer resp.Body.Close()
	kind := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(kind, "image/") {
		return domain.Poster{}, fmt.Errorf("%w: plex poster is %q", domain.ErrUnavailable, kind)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPoster))
	if err != nil {
		return domain.Poster{}, fmt.Errorf("%w: plex poster: %w", domain.ErrUnavailable, err)
	}
	return domain.Poster{ContentType: kind, Body: body}, nil
}

// find resolves an id from Libraries or Titles to the server it is on, among those the person
// can reach, and the key there. Anything else is domain.ErrNotFound.
func (c *Client) find(ctx context.Context, token, id string) (*server, string, error) {
	serverID, key, ok := strings.Cut(id, ".")
	if !ok || !digits(key) {
		return nil, "", domain.ErrNotFound
	}
	servers, err := c.serversFor(ctx, token)
	if err != nil {
		return nil, "", err
	}
	for _, s := range servers {
		if s.id == serverID {
			return s, key, nil
		}
	}
	return nil, "", domain.ErrNotFound
}

type resource struct {
	Name             string       `json:"name"`
	ClientIdentifier string       `json:"clientIdentifier"`
	Provides         string       `json:"provides"`
	Owned            bool         `json:"owned"`
	AccessToken      string       `json:"accessToken"`
	Connections      []connection `json:"connections"`
}

type connection struct {
	URI   string `json:"uri"`
	Local bool   `json:"local"`
	Relay bool   `json:"relay"`
}

// serversFor lists the servers the person can reach, their own first.
func (c *Client) serversFor(ctx context.Context, token string) ([]*server, error) {
	key := sha256.Sum256([]byte(token))
	c.mu.Lock()
	cached, ok := c.servers[key]
	c.mu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.servers, nil
	}
	var resources []resource
	if err := c.call(ctx, http.MethodGet, c.clients+"/api/v2/resources?includeHttps=1&includeRelay=1", token, &resources); err != nil {
		return nil, err
	}
	var servers []*server
	for _, r := range resources {
		if !slices.Contains(strings.Split(r.Provides, ","), "server") || r.ClientIdentifier == "" || r.AccessToken == "" {
			continue
		}
		servers = append(servers, &server{id: r.ClientIdentifier, name: r.Name, owned: r.Owned, token: r.AccessToken, connections: r.Connections})
	}
	slices.SortStableFunc(servers, func(a, b *server) int {
		if a.owned == b.owned {
			return strings.Compare(a.name, b.name)
		}
		if a.owned {
			return -1
		}
		return 1
	})
	c.mu.Lock()
	c.servers[key] = cachedServers{servers: servers, expires: time.Now().Add(cacheFor)}
	c.mu.Unlock()
	return servers, nil
}

// baseOf picks the address to reach the server at: a local one if Homeloom is next to it, else a
// remote one, else Plex's relay. All are tried at once and the most direct that answers wins.
func (c *Client) baseOf(ctx context.Context, s *server) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.base != "" {
		return s.base, nil
	}
	rank := func(cn connection) int {
		switch {
		case cn.Relay:
			return 2
		case cn.Local:
			return 0
		}
		return 1
	}
	conns := slices.Clone(s.connections)
	slices.SortStableFunc(conns, func(a, b connection) int { return rank(a) - rank(b) })
	ok := make([]bool, len(conns))
	var wg sync.WaitGroup
	for i, cn := range conns {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			req, err := c.request(ctx, http.MethodGet, strings.TrimRight(cn.URI, "/")+"/identity", s.token)
			if err != nil {
				return
			}
			resp, err := c.http.Do(req)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
			ok[i] = resp.StatusCode == http.StatusOK
		})
	}
	wg.Wait()
	for i, cn := range conns {
		if ok[i] {
			s.base = strings.TrimRight(cn.URI, "/")
			return s.base, nil
		}
	}
	return "", fmt.Errorf("%w: plex server %q not reachable", domain.ErrUnavailable, s.name)
}

func (c *Client) fromServer(ctx context.Context, s *server, path string, out any) error {
	resp, err := c.serverGet(ctx, s, path, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: plex server: %w", domain.ErrUnavailable, err)
	}
	return nil
}

// serverGet calls the server. A server that no longer lets the person in, or doesn't know the
// key, is domain.ErrNotFound; one that can't be reached forgets its address, so the next call
// looks again.
func (c *Client) serverGet(ctx context.Context, s *server, path, accept string) (*http.Response, error) {
	base, err := c.baseOf(ctx, s)
	if err != nil {
		return nil, err
	}
	req, err := c.request(ctx, http.MethodGet, base+path, s.token)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	resp, err := c.http.Do(req)
	if err != nil {
		s.mu.Lock()
		s.base = ""
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: plex server: %w", domain.ErrUnavailable, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		_ = resp.Body.Close()
		return nil, domain.ErrNotFound
	}
	_ = resp.Body.Close()
	return nil, fmt.Errorf("%w: plex server answered %d", domain.ErrUnavailable, resp.StatusCode)
}

// call talks to plex.tv. A rejected token is domain.ErrUnauthenticated.
func (c *Client) call(ctx context.Context, method, u, token string, out any) error {
	req, err := c.request(ctx, method, u, token)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: plex: %w", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: plex answered %d", domain.ErrUnauthenticated, resp.StatusCode)
	case resp.StatusCode/100 != 2:
		return fmt.Errorf("%w: plex answered %d", domain.ErrUnavailable, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: plex: %w", domain.ErrUnavailable, err)
	}
	return nil
}

// request sets the headers Plex wants on every call; the token goes in a header, never the URL.
func (c *Client) request(ctx context.Context, method, u, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Plex-Product", product)
	req.Header.Set("X-Plex-Client-Identifier", c.clientID)
	if token != "" {
		req.Header.Set("X-Plex-Token", token)
	}
	return req, nil
}

func digits(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
