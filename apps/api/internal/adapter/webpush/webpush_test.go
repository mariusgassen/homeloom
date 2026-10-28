package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func pusher(t *testing.T) *Pusher {
	t.Helper()
	pub, priv, err := GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	p, err := New(pub, priv, "https://homeloom.example.com")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// device is what a browser hands out when it subscribes.
func device(t *testing.T, endpoint string) domain.PushSubscription {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	return domain.PushSubscription{
		Endpoint: endpoint,
		P256dh:   base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
		Lang:     "de",
	}
}

func TestNewRejectsBadKeys(t *testing.T) {
	pub, priv, _ := GenerateKeys()
	for _, keys := range [][2]string{{"", priv}, {pub, ""}, {priv, priv}, {pub, pub}} {
		if _, err := New(keys[0], keys[1], "x"); err == nil {
			t.Fatalf("accepted %q", keys)
		}
	}
}

func TestSend(t *testing.T) {
	status := http.StatusCreated
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.WriteHeader(status)
	}))
	defer srv.Close()
	p := pusher(t)
	sub := device(t, srv.URL)
	n := domain.Notification{Key: "push.revealed", Args: []any{"name", "Bob", "question", "Q"}}

	if err := p.Send(context.Background(), sub, n); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(auth, "vapid t=") || !strings.Contains(auth, ", k="+p.PublicKey()) {
		t.Fatalf("not signed with VAPID: %q", auth)
	}
	status = http.StatusGone
	if err := p.Send(context.Background(), sub, n); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("gone: want ErrNotFound, got %v", err)
	}
	status = http.StatusInternalServerError
	if err := p.Send(context.Background(), sub, n); err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("server error: %v", err)
	}
	srv.Close()
	if err := p.Send(context.Background(), sub, n); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("down: want ErrUnavailable, got %v", err)
	}
}

func TestRenderUsesTheDevicesLanguage(t *testing.T) {
	n := domain.Notification{Key: "push.revealed", Args: []any{"name", "Bob", "question", "Kaffee?"}, URL: "/us", Tag: "t"}
	for lang, want := range map[string]string{"de": "Bob hat auch geantwortet", "en": "Bob answered too", "fr": "Bob answered too"} {
		raw, err := render(domain.PushSubscription{Lang: lang}, n)
		if err != nil {
			t.Fatal(err)
		}
		var m message
		_ = json.Unmarshal(raw, &m)
		if m.Title != want || !strings.Contains(m.Body, "Kaffee?") || m.URL != "/us" || m.Tag != "t" {
			t.Fatalf("%s: %+v", lang, m)
		}
	}
}
