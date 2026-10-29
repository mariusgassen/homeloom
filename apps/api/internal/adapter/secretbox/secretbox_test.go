package secretbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/secretbox"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

const (
	key      = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // 32 bytes
	otherKey = "ZmVkY2JhOTg3NjU0MzIxMGZlZGNiYTk4NzY1NDMyMTA="
)

func TestHomeConnectionsAreStoredSealed(t *testing.T) {
	ctx := context.Background()
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	inner := memory.NewStore()
	sealed := secretbox.NewHomeConnections(inner, box)
	hid := uuid.New()

	if err := sealed.SaveHomeConnection(ctx, domain.HomeConnection{UserID: hid, RefreshToken: "secret-refresh"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := inner.HomeConnection(ctx, hid)
	if raw.RefreshToken == "secret-refresh" || raw.RefreshToken == "" {
		t.Fatalf("stored in the clear: %q", raw.RefreshToken)
	}
	if c, err := sealed.HomeConnection(ctx, hid); err != nil || c.RefreshToken != "secret-refresh" {
		t.Fatalf("open: %+v, %v", c, err)
	}

	// Another key, or a token moved to another person, can't be opened.
	other, err := secretbox.New(otherKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secretbox.NewHomeConnections(inner, other).HomeConnection(ctx, hid); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other key: %v", err)
	}
	moved := uuid.New()
	_ = inner.SaveHomeConnection(ctx, domain.HomeConnection{UserID: moved, RefreshToken: raw.RefreshToken})
	if _, err := sealed.HomeConnection(ctx, moved); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("moved: %v", err)
	}

	for _, bad := range []string{"short", "not base64!", "MDEyMzQ1Njc4OWFiY2RlZg=="} {
		if _, err := secretbox.New(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestWatchConnectionsAreStoredSealed(t *testing.T) {
	ctx := context.Background()
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	inner := memory.NewStore()
	sealed := secretbox.NewWatchConnections(inner, box)
	uid := uuid.New()

	if err := sealed.SaveWatchConnection(ctx, domain.WatchConnection{UserID: uid, Token: "plex-token"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := inner.WatchConnection(ctx, uid)
	if raw.Token == "plex-token" || raw.Token == "" {
		t.Fatalf("stored in the clear: %q", raw.Token)
	}
	if c, err := sealed.WatchConnection(ctx, uid); err != nil || c.Token != "plex-token" {
		t.Fatalf("open: %+v, %v", c, err)
	}

	// A sealed Plex token doesn't open as a Home Assistant one.
	_ = inner.SaveHomeConnection(ctx, domain.HomeConnection{UserID: uid, RefreshToken: raw.Token})
	if _, err := secretbox.NewHomeConnections(inner, box).HomeConnection(ctx, uid); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("opened as a home token: %v", err)
	}
}
