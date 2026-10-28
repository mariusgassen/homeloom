package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestInviteAndJoin(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	svc := service.NewHousehold(store)
	alice, bob, eve := newUser(t, store, "alice"), newUser(t, store, "bob"), newUser(t, store, "eve")
	m, _ := svc.Create(ctx, alice, "Home")

	if _, err := svc.Invite(ctx, eve, m.Household.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member invite: want ErrNotFound, got %v", err)
	}
	inv, err := svc.Invite(ctx, alice, m.Household.ID)
	if err != nil || len(inv.Code) < 43 {
		t.Fatalf("invite: %+v, %v", inv, err)
	}
	if joined, err := svc.Join(ctx, bob, inv.Code); err != nil || joined.Household.ID != m.Household.ID {
		t.Fatalf("join: %+v, %v", joined, err)
	}
	if _, err := svc.Join(ctx, eve, inv.Code); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("reused invite: want ErrNotFound, got %v", err)
	}
	if _, err := svc.Join(ctx, eve, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("empty code: want ErrNotFound, got %v", err)
	}

	late, _ := svc.Invite(ctx, alice, m.Household.ID)
	store.ExpireInvites()
	if _, err := svc.Join(ctx, eve, late.Code); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expired invite: want ErrNotFound, got %v", err)
	}
}

func TestAnswersStaySealedUntilBothAnswered(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	households := service.NewHousehold(store)
	questions := service.NewQuestion(store, store, service.NewPush(store, nil))
	alice, bob := newUser(t, store, "alice"), newUser(t, store, "bob")
	m, _ := households.Create(ctx, alice, "Home")
	store.AddMember(m.Household.ID, bob.ID, domain.RoleMember)

	q, err := questions.Ask(ctx, alice, m.Household.ID, "  What made you laugh today?  ")
	if err != nil || q.Text != "What made you laugh today?" {
		t.Fatalf("ask: %+v, %v", q, err)
	}
	if _, err := questions.Answer(ctx, alice, q.ID, "The cat"); err != nil {
		t.Fatal(err)
	}

	list, _ := questions.List(ctx, bob, m.Household.ID)
	if a := list[0].Answers[0]; !a.Sealed || a.Text != "" {
		t.Fatalf("bob must not read alice's answer yet: %+v", a)
	}
	list, _ = questions.List(ctx, alice, m.Household.ID)
	if a := list[0].Answers[0]; a.Sealed || a.Text != "The cat" {
		t.Fatalf("alice reads her own answer: %+v", a)
	}

	q, err = questions.Answer(ctx, bob, q.ID, "Your face")
	if err != nil || !q.Revealed() {
		t.Fatalf("bob's answer reveals: %+v, %v", q, err)
	}
	for _, a := range q.Answers {
		if a.Sealed || a.Text == "" {
			t.Fatalf("revealed answers are readable: %+v", q.Answers)
		}
	}
	if _, err := questions.Answer(ctx, alice, q.ID, "changed"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("after reveal: want ErrConflict, got %v", err)
	}
}

func TestQuestionsAreMembersOnly(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	households := service.NewHousehold(store)
	questions := service.NewQuestion(store, store, service.NewPush(store, nil))
	alice, eve := newUser(t, store, "alice"), newUser(t, store, "eve")
	m, _ := households.Create(ctx, alice, "Home")
	q, _ := questions.Ask(ctx, alice, m.Household.ID, "Secret?")

	if _, err := questions.List(ctx, eve, m.Household.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("list: want ErrNotFound, got %v", err)
	}
	if _, err := questions.Ask(ctx, eve, m.Household.ID, "Hi"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ask: want ErrNotFound, got %v", err)
	}
	if _, err := questions.Answer(ctx, eve, q.ID, "Hi"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("answer: want ErrNotFound, got %v", err)
	}
	if _, err := questions.Answer(ctx, alice, uuid.New(), "Hi"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown question: want ErrNotFound, got %v", err)
	}
	for _, text := range []string{"", "  ", strings.Repeat("x", domain.MaxQuestionText+1)} {
		if _, err := questions.Ask(ctx, alice, m.Household.ID, text); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("ask %q: want ErrInvalid, got %v", text, err)
		}
	}
	if _, err := questions.Answer(ctx, alice, q.ID, strings.Repeat("x", domain.MaxAnswerText+1)); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("long answer: want ErrInvalid, got %v", err)
	}
}
