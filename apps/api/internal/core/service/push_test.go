package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestSubscribeAcceptsOnlyPushServices(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	push := service.NewPush(store, &memory.Pusher{})
	alice := newUser(t, store, "alice")

	for _, endpoint := range []string{
		"https://web.push.apple.com/QGuQ",
		"https://fcm.googleapis.com/fcm/send/abc",
		"https://updates.push.services.mozilla.com/wpush/v2/abc",
		"https://wns2-par02p.notify.windows.com/w/?token=abc",
	} {
		if err := push.Subscribe(ctx, alice, domain.PushSubscription{Endpoint: endpoint}); err != nil {
			t.Errorf("%s: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{
		"", "not a url", "http://web.push.apple.com/x", "https://web.push.apple.com:8443/x",
		"https://evilpush.apple.com.example.com/x", "https://user@web.push.apple.com/x", "https://localhost/x",
	} {
		if err := push.Subscribe(ctx, alice, domain.PushSubscription{Endpoint: endpoint}); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%q: want ErrInvalid, got %v", endpoint, err)
		}
	}
	if subs, _ := store.PushSubscriptions(ctx, []uuid.UUID{alice.ID}); len(subs) != 4 || subs[0].UserID != alice.ID {
		t.Fatalf("stored for the caller: %+v", subs)
	}
}

func TestPushIsOffWithoutKeys(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	push := service.NewPush(store, nil)
	alice := newUser(t, store, "alice")
	if _, err := push.PublicKey(ctx); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("key: %v", err)
	}
	if err := push.Subscribe(ctx, alice, domain.PushSubscription{Endpoint: "https://web.push.apple.com/x"}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("subscribe: %v", err)
	}
	push.Notify(ctx, []uuid.UUID{alice.ID}, domain.Notification{Key: "push.revealed"})
	push.Wait()
}

func TestRevealNotifiesTheOthers(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	pusher := &memory.Pusher{Gone: map[string]bool{"https://web.push.apple.com/old": true}}
	push := service.NewPush(store, pusher)
	households := service.NewHousehold(store)
	questions := service.NewQuestion(store, store, push, nil)
	alice, bob := newUser(t, store, "alice"), newUser(t, store, "bob")
	m, _ := households.Create(ctx, alice, "Home")
	store.AddMember(m.Household.ID, bob.ID, domain.RoleMember)
	for _, sub := range []struct {
		who      domain.User
		endpoint string
	}{{alice, "https://web.push.apple.com/alice"}, {bob, "https://web.push.apple.com/bob"}, {alice, "https://web.push.apple.com/old"}} {
		if err := push.Subscribe(ctx, sub.who, domain.PushSubscription{Endpoint: sub.endpoint, Lang: "de"}); err != nil {
			t.Fatal(err)
		}
	}

	q, _ := questions.Ask(ctx, bob, m.Household.ID, "Coffee or tea?")
	_, _ = questions.Answer(ctx, bob, q.ID, "Tea")
	_, _ = questions.Answer(ctx, bob, q.ID, "Coffee")
	push.Wait()
	if sent := pusher.Sent(); len(sent) != 0 {
		t.Fatalf("nothing is revealed yet: %+v", sent)
	}

	if _, err := questions.Answer(ctx, alice, q.ID, "Both"); err != nil {
		t.Fatal(err)
	}
	push.Wait()
	sent := pusher.Sent()
	if len(sent) != 1 || sent[0].Subscription.Endpoint != "https://web.push.apple.com/bob" {
		t.Fatalf("only bob hears about it: %+v", sent)
	}
	if n := sent[0].Notification; n.Key != "push.revealed" || n.URL != "/us?show=questions" || n.Args[1] != "alice" || n.Args[3] != "Coffee or tea?" {
		t.Fatalf("notification: %+v", n)
	}

	// Alice's expired device is dropped once a notification for her bounces.
	push.Notify(ctx, []uuid.UUID{alice.ID}, domain.Notification{Key: "push.revealed"})
	push.Wait()
	if subs, _ := store.PushSubscriptions(ctx, []uuid.UUID{alice.ID}); len(subs) != 1 || subs[0].Endpoint != "https://web.push.apple.com/alice" {
		t.Fatalf("expired device kept: %+v", subs)
	}
}

func TestMutedTopicsAreSkipped(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	pusher := &memory.Pusher{}
	push := service.NewPush(store, pusher)
	alice, bob := newUser(t, store, "alice"), newUser(t, store, "bob")
	for _, u := range []domain.User{alice, bob} {
		if err := push.Subscribe(ctx, u, domain.PushSubscription{Endpoint: "https://web.push.apple.com/" + u.Name}); err != nil {
			t.Fatal(err)
		}
	}

	if err := push.SetTopic(ctx, alice, "nope", false); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown topic: %v", err)
	}
	if err := push.SetTopic(ctx, alice, domain.PushTopicReminders, false); err != nil {
		t.Fatal(err)
	}
	topics, err := push.Topics(ctx, alice)
	if err != nil || len(topics) != len(domain.PushTopics) {
		t.Fatalf("topics: %+v, %v", topics, err)
	}
	for _, s := range topics {
		if s.Enabled != (s.Topic != domain.PushTopicReminders) {
			t.Fatalf("only reminders are off: %+v", s)
		}
	}

	both := []uuid.UUID{alice.ID, bob.ID}
	push.Notify(ctx, both, domain.Notification{Key: "push.reminder"})
	push.Notify(ctx, both, domain.Notification{Key: "push.revealed"})
	push.Wait()
	reminders, other := 0, 0
	for _, s := range pusher.Sent() {
		switch {
		case s.Notification.Key == "push.reminder" && s.Subscription.UserID == bob.ID:
			reminders++
		case s.Notification.Key == "push.reminder":
			t.Fatalf("alice muted reminders: %+v", s)
		default:
			other++
		}
	}
	if reminders != 1 || other != 2 {
		t.Fatalf("bob's reminder and both reveals: %d, %d", reminders, other)
	}

	if err := push.SetTopic(ctx, alice, domain.PushTopicReminders, true); err != nil {
		t.Fatal(err)
	}
	push.Notify(ctx, []uuid.UUID{alice.ID}, domain.Notification{Key: "push.reminder"})
	push.Wait()
	if len(pusher.Sent()) != 4 {
		t.Fatalf("on again: %+v", pusher.Sent())
	}
}

func TestEveryPushKeyHasATopic(t *testing.T) {
	for _, k := range []string{"push.reminder", "push.event", "push.briefing", "push.gift_birthday", "push.watch_match", "push.last_month", "push.call", "push.chores_due", "push.revealed"} {
		if domain.ValidPushTopic(domain.Notification{Key: k}.Topic()) == false {
			t.Errorf("%s has no topic", k)
		}
	}
}
