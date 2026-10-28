package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestCallRingsTheOthersOnlyWhenNobodyIsThere(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	pusher := &memory.Pusher{}
	push := service.NewPush(store, pusher)
	server := memory.NewCallServer()
	calls := service.NewCall(store, server, push)
	alice, bob, eve := newUser(t, store, "alice"), newUser(t, store, "bob"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	store.AddMember(m.Household.ID, bob.ID, domain.RoleMember)
	for _, u := range []domain.User{alice, bob} {
		if err := push.Subscribe(ctx, u, domain.PushSubscription{Endpoint: "https://web.push.apple.com/" + u.Name}); err != nil {
			t.Fatal(err)
		}
	}

	c, err := calls.Join(ctx, alice, m.Household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Room != "household-"+m.Household.ID.String() || c.Token != alice.ID.String()+"@"+c.Room || c.URL != "wss://calls.example" {
		t.Fatalf("call: %+v", c)
	}
	push.Wait()
	sent := pusher.Sent()
	if len(sent) != 1 || sent[0].Subscription.Endpoint != "https://web.push.apple.com/bob" {
		t.Fatalf("only bob is rung: %+v", sent)
	}
	if n := sent[0].Notification; n.Key != "push.call" || n.Args[1] != "alice" || n.URL != "/call?from="+alice.ID.String() || n.Tag != c.Room {
		t.Fatalf("notification: %+v", n)
	}

	// Bob answers while Alice is in the call: nobody rings again.
	server.Enter(c.Room, alice.ID.String())
	if _, err := calls.Join(ctx, bob, m.Household.ID); err != nil {
		t.Fatal(err)
	}
	push.Wait()
	if sent := pusher.Sent(); len(sent) != 1 {
		t.Fatalf("answering rang: %+v", sent)
	}

	if _, err := calls.Join(ctx, eve, m.Household.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}
	server.Down = true
	if _, err := calls.Join(ctx, bob, m.Household.ID); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("server down: %v", err)
	}
	if _, err := service.NewCall(store, nil, push).Join(ctx, bob, m.Household.ID); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("not set up: %v", err)
	}
}
