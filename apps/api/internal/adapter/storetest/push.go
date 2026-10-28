package storetest

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// PushSubscriptions: one row per endpoint, which follows the account that saved it last; a
// person deletes only their own; an expired endpoint goes whoever owns it.
func PushSubscriptions(t *testing.T, s interface {
	Store
	port.PushSubscriptionRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	phone := "https://web.push.apple.com/" + uuid.NewString()
	laptop := "https://fcm.googleapis.com/fcm/send/" + uuid.NewString()

	save := func(u domain.User, endpoint, lang string) {
		t.Helper()
		if err := s.SavePushSubscription(ctx, domain.PushSubscription{UserID: u.ID, Endpoint: endpoint, P256dh: "p", Auth: "a", Lang: lang}); err != nil {
			t.Fatal(err)
		}
	}
	count := func(u domain.User) int {
		t.Helper()
		subs, err := s.PushSubscriptions(ctx, []uuid.UUID{u.ID})
		if err != nil {
			t.Fatal(err)
		}
		return len(subs)
	}

	save(alice, phone, "en")
	save(alice, phone, "de")
	save(alice, laptop, "en")
	subs, err := s.PushSubscriptions(ctx, []uuid.UUID{alice.ID, bob.ID})
	if err != nil || len(subs) != 2 {
		t.Fatalf("want alice's two devices, got %+v, %v", subs, err)
	}
	for _, sub := range subs {
		if sub.Endpoint == phone && (sub.Lang != "de" || sub.UserID != alice.ID || sub.P256dh != "p" || sub.Auth != "a") {
			t.Fatalf("saving again replaces: %+v", sub)
		}
	}

	save(bob, phone, "en")
	if count(alice) != 1 || count(bob) != 1 {
		t.Fatalf("the phone moved to bob: alice %d, bob %d", count(alice), count(bob))
	}
	if err := s.DeletePushSubscription(ctx, alice.ID, phone); err != nil || count(bob) != 1 {
		t.Fatalf("alice can't delete bob's: %v, %d", err, count(bob))
	}
	if err := s.DeletePushSubscription(ctx, alice.ID, laptop); err != nil || count(alice) != 0 {
		t.Fatalf("alice deletes her own: %v, %d", err, count(alice))
	}
	if err := s.DeletePushEndpoint(ctx, phone); err != nil || count(bob) != 0 {
		t.Fatalf("expired endpoint: %v, %d", err, count(bob))
	}
	if subs, err := s.PushSubscriptions(ctx, nil); err != nil || len(subs) != 0 {
		t.Fatalf("no users: %+v, %v", subs, err)
	}
}
