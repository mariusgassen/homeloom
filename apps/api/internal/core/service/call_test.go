package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestCalls(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	pusher := &memory.Pusher{}
	push := service.NewPush(store, pusher)
	server := memory.NewCallServer()
	calls := service.NewCall(store, memory.NewSessions(), server, push)
	alice, bob, carol, eve := newUser(t, store, "alice"), newUser(t, store, "bob"), newUser(t, store, "carol"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	h := m.Household.ID
	store.AddMember(h, bob.ID, domain.RoleMember)
	store.AddMember(h, carol.ID, domain.RoleMember)
	for _, u := range []domain.User{alice, bob, carol} {
		if err := push.Subscribe(ctx, u, domain.PushSubscription{Endpoint: "https://web.push.apple.com/" + u.Name}); err != nil {
			t.Fatal(err)
		}
	}
	rung := func() []string {
		push.Wait()
		var out []string
		for _, p := range pusher.Sent() {
			out = append(out, strings.TrimPrefix(p.Subscription.Endpoint, "https://web.push.apple.com/"))
		}
		return out
	}

	// Alice calls Bob: only Bob rings, and the push opens this call.
	c, err := calls.Start(ctx, alice, h, []uuid.UUID{bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != alice.ID.String()+"/alice@"+c.Room || c.URL != "wss://calls.example" || !strings.Contains(c.Room, h.String()) {
		t.Fatalf("call: %+v", c)
	}
	if got := rung(); len(got) != 1 || got[0] != "bob" {
		t.Fatalf("rung: %v", got)
	}
	if n := pusher.Sent()[0].Notification; n.Key != "push.call" || n.Args[1] != "alice" || n.URL != "/call/"+c.ID.String()+"?from="+alice.ID.String() || n.Tag != c.Room {
		t.Fatalf("notification: %+v", n)
	}

	// Bob answers into the same room.
	answer, err := calls.Join(ctx, bob, h, c.ID)
	if err != nil || answer.Room != c.Room || answer.ID != c.ID {
		t.Fatalf("answer: %+v %v", answer, err)
	}
	server.Enter(c.Room, alice.ID.String())
	server.Enter(c.Room, bob.ID.String())

	// Bob adds Carol; Alice, already in the call, isn't rung again.
	if err := calls.Invite(ctx, bob, h, c.ID, []uuid.UUID{carol.ID, alice.ID}); err != nil {
		t.Fatal(err)
	}
	if got := rung(); len(got) != 2 || got[1] != "carol" {
		t.Fatalf("rung: %v", got)
	}

	// A guest link lets one device in, under the guest's name and a fresh identity each time.
	link, err := calls.GuestLink(ctx, alice, h, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pass, err := calls.GuestPass(ctx, link.Token); err != nil || pass.InvitedBy != "alice" {
		t.Fatalf("pass: %+v %v", pass, err)
	}
	g1, err := calls.JoinAsGuest(ctx, link.Token, "phone", "  Grandma ")
	if err != nil || g1.Room != c.Room || !strings.HasPrefix(g1.Token, "guest-") || !strings.HasSuffix(g1.Token, "/Grandma@"+c.Room) {
		t.Fatalf("guest: %+v %v", g1, err)
	}
	if g2, err := calls.JoinAsGuest(ctx, link.Token, "phone", "Grandma"); err != nil || g2.Token == g1.Token {
		t.Fatalf("coming back: %+v %v", g2, err)
	}
	if _, err := calls.JoinAsGuest(ctx, link.Token, "laptop", "Mallory"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("forwarded link: %v", err)
	}
	if _, err := calls.JoinAsGuest(ctx, link.Token, "phone", " "); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("no name: %v", err)
	}
	if _, err := calls.JoinAsGuest(ctx, "unknown", "phone", "Mallory"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown link: %v", err)
	}

	// Outsiders and odd invites.
	if _, err := calls.Start(ctx, alice, h, []uuid.UUID{alice.ID}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("calling yourself: %v", err)
	}
	if _, err := calls.Start(ctx, alice, h, []uuid.UUID{eve.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("calling an outsider: %v", err)
	}
	if _, err := calls.Join(ctx, eve, h, c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider joining: %v", err)
	}
	if _, err := calls.GuestLink(ctx, eve, h, c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider making a guest link: %v", err)
	}
	other, _ := service.NewHousehold(store).Create(ctx, eve, "Elsewhere")
	if own, _ := calls.Join(ctx, eve, other.Household.ID, c.ID); own.Room == c.Room {
		t.Fatal("a call id opens another household's room")
	}

	// Talking to Homeloom: a private room, and the agent gets a key that acts as Alice.
	agents := service.NewAgentAccess(store, memory.NewSessions(), store)
	voice := service.NewCall(store, memory.NewSessions(), server, push).WithVoiceAgent("homeloom", agents)
	vc, err := voice.TalkToHomeloom(ctx, alice, h, "de")
	if err != nil || !strings.HasPrefix(vc.Room, "homeloom-") || !strings.Contains(vc.Room, alice.ID.String()) {
		t.Fatalf("voice call: %+v %v", vc, err)
	}
	var meta service.VoiceCall
	if err := json.Unmarshal([]byte(server.Dispatched["homeloom@"+vc.Room]), &meta); err != nil || meta.Lang != "de" || meta.Name != "alice" {
		t.Fatalf("dispatch: %+v %v", meta, err)
	}
	if grant, err := agents.Authenticate(ctx, meta.Key); err != nil || grant.User.ID != alice.ID || grant.HouseholdID != h {
		t.Fatalf("agent key: %+v %v", grant, err)
	}
	if _, err := voice.TalkToHomeloom(ctx, eve, h, "en"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider talking to homeloom: %v", err)
	}
	if _, err := calls.TalkToHomeloom(ctx, alice, h, "en"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("without the agent: %v", err)
	}

	server.Down = true
	if err := calls.Invite(ctx, bob, h, c.ID, []uuid.UUID{carol.ID}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("server down: %v", err)
	}
	if _, err := service.NewCall(store, memory.NewSessions(), nil, push).Start(ctx, bob, h, []uuid.UUID{alice.ID}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("not set up: %v", err)
	}
}
