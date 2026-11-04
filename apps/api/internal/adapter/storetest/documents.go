package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Documents: private until shared, only the owner changes or deletes, and no reaching into another household.
func Documents(t *testing.T, s interface {
	Store
	port.DocumentRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	home, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, "Elsewhere", bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	join(t, s, home.Household.ID, alice, bob)
	hh := home.Household.ID

	lease, err := s.CreateDocument(ctx, domain.Document{ID: uuid.New(), HouseholdID: hh, OwnerID: alice.ID, Name: "lease.pdf", ContentType: "application/pdf", Size: 12})
	if err != nil {
		t.Fatal(err)
	}
	if lease.Shared || lease.CreatedAt.IsZero() {
		t.Fatalf("a new document is private: %+v", lease)
	}
	if ds, err := s.Documents(ctx, hh, alice.ID, 10); err != nil || len(ds) != 1 {
		t.Fatalf("alice's own: %v %v", ds, err)
	}
	if ds, err := s.Documents(ctx, hh, bob.ID, 10); err != nil || len(ds) != 0 {
		t.Fatalf("bob sees no private document: %v %v", ds, err)
	}
	if _, err := s.GetDocument(ctx, hh, bob.ID, lease.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob gets a private document: %v", err)
	}
	if _, err := s.UpdateDocument(ctx, hh, bob.ID, lease.ID, "mine", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob changes it: %v", err)
	}

	shared, err := s.UpdateDocument(ctx, hh, alice.ID, lease.ID, "Lease 2026", true)
	if err != nil || !shared.Shared || shared.Name != "Lease 2026" {
		t.Fatalf("share: %+v %v", shared, err)
	}
	if got, err := s.GetDocument(ctx, hh, bob.ID, lease.ID); err != nil || got.Name != "Lease 2026" {
		t.Fatalf("bob reads a shared document: %+v %v", got, err)
	}
	if _, err := s.UpdateDocument(ctx, hh, bob.ID, lease.ID, "mine", false); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a shared document is still only the owner's to change: %v", err)
	}
	if err := s.DeleteDocument(ctx, hh, bob.ID, lease.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob deletes it: %v", err)
	}
	if _, err := s.GetDocument(ctx, other.Household.ID, alice.ID, lease.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another household's lookup: %v", err)
	}
	if err := s.DeleteDocument(ctx, hh, alice.ID, lease.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDocument(ctx, hh, alice.ID, lease.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}
