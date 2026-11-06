// Package bigdatacloud names the town a position is in with BigDataCloud's free reverse
// geocoding, which needs no key.
package bigdatacloud

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
	baseURL = "https://api.bigdatacloud.net"
	timeout = 5 * time.Second
	maxBody = 1 << 20
)

// Client asks BigDataCloud for the name of a place.
type Client struct {
	http *http.Client
	base string
}

var _ port.PlaceNamer = (*Client)(nil)

func New() *Client { return &Client{http: &http.Client{Timeout: timeout}, base: baseURL} }

// NewAt points the client at another host, for tests.
func NewAt(base string) *Client { return &Client{http: &http.Client{Timeout: timeout}, base: base} }

func (c *Client) PlaceName(ctx context.Context, at domain.Coords, lang string) (string, error) {
	q := url.Values{"latitude": {fmt.Sprint(at.Latitude)}, "longitude": {fmt.Sprint(at.Longitude)}}
	if base, _, _ := strings.Cut(lang, "-"); base != "" {
		q.Set("localityLanguage", base)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/data/reverse-geocode-client?"+q.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: bigdatacloud: %w", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: bigdatacloud answered %d", domain.ErrUnavailable, resp.StatusCode)
	}
	var out struct {
		City     string `json:"city"`
		Locality string `json:"locality"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&out); err != nil {
		return "", fmt.Errorf("%w: bigdatacloud answered no JSON: %w", domain.ErrUnavailable, err)
	}
	if out.City != "" {
		return out.City, nil
	}
	return out.Locality, nil
}
