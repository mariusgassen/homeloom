package livekit_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/livekit"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

const secret = "0123456789abcdef0123456789abcdef"

type videoClaims struct {
	jwt.Claims
	Name  string         `json:"name"`
	Video map[string]any `json:"video"`
}

func parse(t *testing.T, token string) videoClaims {
	t.Helper()
	tok, err := jwt.ParseSigned(token, []jose.SignatureAlgorithm{jose.HS256})
	if err != nil {
		t.Fatal(err)
	}
	var c videoClaims
	if err := tok.Claims([]byte(secret), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTokenJoinsOneRoom(t *testing.T) {
	s, err := livekit.New("wss://home.example", "http://livekit:7880", "key", secret)
	if err != nil {
		t.Fatal(err)
	}
	u := domain.User{ID: uuid.New(), Name: "Alice"}
	token, err := s.Token(u.ID.String(), u.Name, "household-1")
	if err != nil {
		t.Fatal(err)
	}
	c := parse(t, token)
	if c.Issuer != "key" || c.Subject != u.ID.String() || c.Name != "Alice" || c.Expiry == nil {
		t.Fatalf("claims: %+v", c)
	}
	if c.Video["room"] != "household-1" || c.Video["roomJoin"] != true || c.Video["canPublish"] != true || c.Video["roomAdmin"] != nil {
		t.Fatalf("grant: %+v", c.Video)
	}
}

func TestShortSecretIsRefused(t *testing.T) {
	if _, err := livekit.New("wss://home.example", "http://livekit:7880", "key", "short"); err == nil {
		t.Fatal("want an error")
	}
}

func TestParticipants(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		var body struct{ Room string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case r.URL.Path != "/twirp/livekit.RoomService/ListParticipants":
			http.NotFound(w, r)
		case body.Room == "empty":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found","msg":"requested room does not exist"}`))
		case body.Room == "broken":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte(`{"participants":[{"identity":"a","name":"Alice"},{"identity":"b"}]}`))
		}
	}))
	defer srv.Close()
	s, err := livekit.New("wss://home.example", srv.URL+"/", "key", secret)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	ids, err := s.Participants(ctx, "household-1")
	if err != nil || strings.Join(ids, ",") != "a,b" {
		t.Fatalf("participants: %v %v", ids, err)
	}
	c := parse(t, strings.TrimPrefix(auth, "Bearer "))
	if c.Video["room"] != "household-1" || c.Video["roomAdmin"] != true || c.Video["roomJoin"] != nil {
		t.Fatalf("admin grant: %+v", c.Video)
	}
	if ids, err := s.Participants(ctx, "empty"); err != nil || len(ids) != 0 {
		t.Fatalf("a room nobody joined: %v %v", ids, err)
	}
	if _, err := s.Participants(ctx, "broken"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("server error: %v", err)
	}
	srv.Close()
	if _, err := s.Participants(ctx, "household-1"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("server down: %v", err)
	}
}
