// Package livekit hands out access tokens for the self-hosted LiveKit server and asks it who is in
// a room. Tokens are the JWTs LiveKit expects, signed with the API key's secret.
package livekit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

const (
	// tokenTTL only needs to cover connecting: LiveKit refreshes the token of a connected client.
	tokenTTL = 10 * time.Minute
	// callTimeout bounds one request to the server API.
	callTimeout = 5 * time.Second
)

type Server struct {
	publicURL string // where clients connect, wss://...
	apiURL    string // where the API reaches the server, http://homeloom-livekit:7880
	key       string
	signer    jose.Signer
	client    *http.Client
}

// New takes the URL clients connect to, the URL the API reaches the server at, and the API key
// pair from the server's keys.
func New(publicURL, apiURL, key, secret string) (*Server, error) {
	if len(secret) < 32 {
		return nil, errors.New("LIVEKIT_API_SECRET: want at least 32 characters")
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte(secret)}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return nil, err
	}
	return &Server{
		publicURL: publicURL,
		apiURL:    strings.TrimRight(apiURL, "/"),
		key:       key,
		signer:    signer,
		client:    &http.Client{Timeout: callTimeout},
	}, nil
}

func (s *Server) URL() string { return s.publicURL }

// grant is LiveKit's video grant.
type grant struct {
	Room           string `json:"room"`
	RoomJoin       bool   `json:"roomJoin,omitempty"`
	RoomAdmin      bool   `json:"roomAdmin,omitempty"`
	CanPublish     *bool  `json:"canPublish,omitempty"`
	CanSubscribe   *bool  `json:"canSubscribe,omitempty"`
	CanPublishData *bool  `json:"canPublishData,omitempty"`
}

type claims struct {
	jwt.Claims
	Name  string `json:"name,omitempty"`
	Video grant  `json:"video"`
}

func (s *Server) Token(user domain.User, room string) (string, error) {
	yes := true
	return s.sign(user.ID.String(), user.Name, grant{Room: room, RoomJoin: true, CanPublish: &yes, CanSubscribe: &yes, CanPublishData: &yes})
}

func (s *Server) sign(identity, name string, g grant) (string, error) {
	now := time.Now()
	c := claims{
		Claims: jwt.Claims{Issuer: s.key, Subject: identity, NotBefore: jwt.NewNumericDate(now), Expiry: jwt.NewNumericDate(now.Add(tokenTTL))},
		Name:   name,
		Video:  g,
	}
	return jwt.Signed(s.signer).Claims(c).Serialize()
}

func (s *Server) Participants(ctx context.Context, room string) ([]string, error) {
	token, err := s.sign("homeloom-api", "", grant{Room: room, RoomAdmin: true})
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]string{"room": room})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL+"/twirp/livekit.RoomService/ListParticipants", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: livekit: %w", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, nil // nobody has joined, so the room doesn't exist yet
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: livekit answered %s", domain.ErrUnavailable, resp.Status)
	}
	var out struct {
		Participants []struct {
			Identity string `json:"identity"`
		} `json:"participants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: livekit: %w", domain.ErrUnavailable, err)
	}
	ids := make([]string, len(out.Participants))
	for i, p := range out.Participants {
		ids[i] = p.Identity
	}
	return ids, nil
}
