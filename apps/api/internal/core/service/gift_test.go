package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

type giftFixture struct {
	*choreFixture
	gifts *service.Gifts
}

func newGiftFixture(t *testing.T) *giftFixture {
	t.Helper()
	f := &giftFixture{choreFixture: newChoreFixture(t)}
	f.gifts = f.newGifts()
	return f
}

func (f *giftFixture) newGifts() *service.Gifts {
	g := service.NewGifts(f.store, f.store, f.store, f.push, time.UTC)
	g.SetClock(func() time.Time { return f.now })
	return g
}

func TestGiftIdeasAreOnlyTheirAuthors(t *testing.T) {
	ctx := context.Background()
	f := newGiftFixture(t)

	idea, err := f.gifts.Add(ctx, f.alice, f.household, f.bob.ID, "  Climbing shoes  ")
	if err != nil || idea.Text != "Climbing shoes" || idea.AuthorID != f.alice.ID || idea.ForID != f.bob.ID {
		t.Fatalf("add: %+v, %v", idea, err)
	}
	if _, err := f.gifts.Add(ctx, f.bob, f.household, f.alice.ID, "Plant"); err != nil {
		t.Fatal(err)
	}

	// Bob sees his own idea for Alice and nothing of Alice's, however he asks.
	g, err := f.gifts.Overview(ctx, f.bob, f.household)
	if err != nil || len(g.Ideas) != 1 || g.Ideas[0].Text != "Plant" || len(g.People) != 1 || g.People[0].Ideas != 1 {
		t.Fatalf("bob's overview: %+v, %v", g, err)
	}
	if _, err := f.gifts.SetGiven(ctx, f.bob, f.household, idea.ID, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob ticks Alice's idea: %v", err)
	}
	if err := f.gifts.Delete(ctx, f.bob, f.household, idea.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob deletes Alice's idea: %v", err)
	}
	if g, _ := f.gifts.Overview(ctx, f.alice, f.household); len(g.Ideas) != 1 || g.Ideas[0].Text != "Climbing shoes" || g.Ideas[0].GivenAt != nil {
		t.Fatalf("Alice's idea untouched: %+v", g.Ideas)
	}

	// An outsider gets nothing, and members only write for other members.
	if _, err := f.gifts.Overview(ctx, f.eve, f.household); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}
	if _, err := f.gifts.Add(ctx, f.alice, f.household, f.alice.ID, "Me"); !errors.Is(err, domain.ErrGiftFor) {
		t.Fatalf("for oneself: %v", err)
	}
	if _, err := f.gifts.Add(ctx, f.alice, f.household, f.eve.ID, "Eve"); !errors.Is(err, domain.ErrGiftFor) {
		t.Fatalf("for an outsider: %v", err)
	}
	if _, err := f.gifts.Add(ctx, f.alice, f.household, f.bob.ID, "  "); !errors.As(err, new(domain.TextError)) {
		t.Fatalf("empty: %v", err)
	}

	// Given ideas stop counting and can be taken back; the author deletes their own.
	if i, err := f.gifts.SetGiven(ctx, f.alice, f.household, idea.ID, true); err != nil || i.GivenAt == nil {
		t.Fatalf("given: %+v, %v", i, err)
	}
	if g, _ := f.gifts.Overview(ctx, f.alice, f.household); g.People[0].Ideas != 0 {
		t.Fatalf("a given idea isn't open any more: %+v", g.People)
	}
	if err := f.gifts.Delete(ctx, f.alice, f.household, idea.ID); err != nil {
		t.Fatal(err)
	}
}

func TestBirthdaysComeSoonestFirst(t *testing.T) {
	ctx := context.Background()
	f := newGiftFixture(t) // 28 October 2026
	if err := f.gifts.SetBirthday(ctx, f.bob, &domain.Birthday{Month: time.November, Day: 4}); err != nil {
		t.Fatal(err)
	}
	if err := f.gifts.SetBirthday(ctx, f.alice, &domain.Birthday{Month: time.February, Day: 30}); !errors.Is(err, domain.ErrBirthday) {
		t.Fatalf("30 February: %v", err)
	}
	if err := f.gifts.SetBirthday(ctx, f.alice, &domain.Birthday{Month: time.February, Day: 29}); err != nil {
		t.Fatal(err)
	}

	g, err := f.gifts.Overview(ctx, f.alice, f.household)
	if err != nil || g.MyBirthday == nil || g.MyBirthday.Day != 29 || len(g.People) != 1 || g.People[0].Days != 7 || g.People[0].Birthday == nil {
		t.Fatalf("alice's overview: %+v, %v", g, err)
	}
	// Without a year of 29 February, the birthday falls on the 28th: 2027 is a common year.
	g, _ = f.gifts.Overview(ctx, f.bob, f.household)
	if next := g.People[0].Next; next.Format("2006-01-02") != "2027-02-28" || g.People[0].Days != 123 {
		t.Fatalf("29 February in 2027: %v, %d days", g.People[0].Next, g.People[0].Days)
	}
	if err := f.gifts.SetBirthday(ctx, f.bob, nil); err != nil {
		t.Fatal(err)
	}
	if g, _ := f.gifts.Overview(ctx, f.alice, f.household); g.People[0].Birthday != nil {
		t.Fatalf("removed: %+v", g.People[0])
	}
}

func TestBirthdayNudgeGoesToEachAuthorWithOnlyTheirOwnCount(t *testing.T) {
	ctx := context.Background()
	f := newGiftFixture(t)
	f.gifts.SetClock(func() time.Time { return time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC) })
	// Bob's birthday is in a week. Alice has two ideas for him; Bob's own birthday is not his to plan.
	if err := f.gifts.SetBirthday(ctx, f.bob, &domain.Birthday{Month: time.November, Day: 4}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Shoes", "Chalk bag"} {
		if _, err := f.gifts.Add(ctx, f.alice, f.household, f.bob.ID, text); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.gifts.NotifyDaily(ctx); err != nil {
		t.Fatal(err)
	}
	f.push.Wait()
	sent := f.pusher.Sent()
	if len(sent) != 1 || !strings.HasSuffix(sent[0].Subscription.Endpoint, "alice") || sent[0].Notification.Key != "push.gift_birthday" {
		t.Fatalf("nudge: %+v", sent)
	}
	if args := sent[0].Notification.Args; !slices.Equal(args, []any{"name", "bob", "count", 2}) {
		t.Fatalf("args: %v", args)
	}
	// Once per day, and not before nine.
	if err := f.gifts.NotifyDaily(ctx); err != nil {
		t.Fatal(err)
	}
	f.push.Wait()
	if n := len(f.pusher.Sent()); n != 1 {
		t.Fatalf("second run sent %d", n)
	}
}

func TestAssistantGiftToolsOnlyTouchThePersonsOwnIdeas(t *testing.T) {
	ctx := context.Background()
	f := newAssistantFixture(t)
	run := func(u domain.User, name, input string) (string, error) {
		return f.assistant.RunTool(ctx, u, f.household, domain.ToolCall{Name: name, Input: json.RawMessage(input)})
	}
	if _, err := run(f.alice, "list_gift_ideas", `{}`); err == nil || err.Error() != "that part of the app isn't available right now" {
		t.Fatalf("without gifts: %v", err)
	}
	gifts := service.NewGifts(f.store, f.store, f.store, nil, time.UTC)
	f.assistant.WithGifts(gifts)

	if out, err := run(f.alice, "add_gift_idea", `{"for":"`+f.bob.Name+`","text":"Climbing shoes"}`); err != nil || !strings.Contains(out, "Climbing shoes") {
		t.Fatalf("add: %s, %v", out, err)
	}
	if _, err := run(f.alice, "add_gift_idea", `{"for":"`+f.alice.Name+`","text":"Me"}`); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("for oneself: %v", err)
	}
	if out, err := run(f.alice, "list_gift_ideas", `{}`); err != nil || !strings.Contains(out, "Climbing shoes") || !strings.Contains(out, f.bob.Name) {
		t.Fatalf("alice lists: %s, %v", out, err)
	}
	out, err := run(f.bob, "list_gift_ideas", `{}`)
	if err != nil || strings.Contains(out, "Climbing shoes") || !strings.Contains(out, `"ideas":[]`) {
		t.Fatalf("bob must not see Alice's idea: %s, %v", out, err)
	}
	if _, err := run(f.eve, "list_gift_ideas", `{}`); err == nil || err.Error() != "not found" {
		t.Fatalf("outsider: %v", err)
	}
	if got := ideasOf(t, f.store, f.household, f.bob.ID); len(got) != 0 {
		t.Fatalf("bob has no ideas: %+v", got)
	}
}

func ideasOf(t *testing.T, s *memory.Store, hh, author uuid.UUID) []domain.GiftIdea {
	t.Helper()
	ideas, err := s.GiftIdeas(context.Background(), hh, author, 100)
	if err != nil {
		t.Fatal(err)
	}
	return ideas
}
