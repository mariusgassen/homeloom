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

const key = "0123456789abcdef0123456789abcdef"

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
	other, _ := secretbox.New(key + "x")
	if _, err := secretbox.NewHomeConnections(inner, other).HomeConnection(ctx, hid); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other key: %v", err)
	}
	moved := uuid.New()
	_ = inner.SaveHomeConnection(ctx, domain.HomeConnection{UserID: moved, RefreshToken: raw.RefreshToken})
	if _, err := sealed.HomeConnection(ctx, moved); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("moved: %v", err)
	}

	if _, err := secretbox.New("short"); err == nil {
		t.Fatal("accepted a short key")
	}
}
