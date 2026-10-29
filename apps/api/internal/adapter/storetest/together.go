// Package storetest holds behaviour every store must share, run against each implementation
// (memory always, Postgres when a test database is available).
package storetest

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

type Store interface {
	port.UserRepository
	port.HouseholdRepository
	port.QuestionRepository
}

func user(t *testing.T, s Store, name string) domain.User {
	t.Helper()
	u, err := s.UpsertByIdentity(context.Background(), domain.Identity{Subject: "sub-" + uuid.NewString(), Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func hash(code string) []byte {
	sum := sha256.Sum256([]byte(code))
	return sum[:]
}

// Invites: one use, expiry, existing members keep the invite unused.
func Invites(t *testing.T, s Store) {
	ctx := context.Background()
	alice, bob, eve := user(t, s, "Alice"), user(t, s, "Bob"), user(t, s, "Eve")
	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	code := uuid.NewString()
	if err := s.CreateInvite(ctx, m.Household.ID, alice.ID, hash(code), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if got, err := s.AcceptInvite(ctx, hash(code), alice.ID); err != nil || got.Role != domain.RoleOwner {
		t.Fatalf("owner accepting own invite: %+v, %v", got, err)
	}
	got, err := s.AcceptInvite(ctx, hash(code), bob.ID)
	if err != nil || got.Household.ID != m.Household.ID || got.Role != domain.RoleMember {
		t.Fatalf("bob joins: %+v, %v", got, err)
	}
	if _, err := s.AcceptInvite(ctx, hash(code), eve.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("used invite: want ErrNotFound, got %v", err)
	}
	if _, err := s.AcceptInvite(ctx, hash("unknown"), eve.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown invite: want ErrNotFound, got %v", err)
	}

	expired := uuid.NewString()
	if err := s.CreateInvite(ctx, m.Household.ID, alice.ID, hash(expired), time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptInvite(ctx, hash(expired), eve.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expired invite: want ErrNotFound, got %v", err)
	}
	if role, err := s.Role(ctx, m.Household.ID, bob.ID); err != nil || role != domain.RoleMember {
		t.Fatalf("bob's role: %v, %v", role, err)
	}
}

// Questions: reveal once every member answered, at least two; answers fixed afterwards;
// simultaneous last answers still reveal.
func Questions(t *testing.T, s Store) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	solo, err := s.CreateQuestion(ctx, m.Household.ID, alice.ID, "Alone?")
	if err != nil {
		t.Fatal(err)
	}
	if q, err := s.Answer(ctx, solo.ID, alice.ID, "yes"); err != nil || q.Revealed() {
		t.Fatalf("one member must not reveal: %+v, %v", q, err)
	}

	join(t, s, m.Household.ID, alice, bob)
	q, err := s.CreateQuestion(ctx, m.Household.ID, bob.ID, "Best day this week?")
	if err != nil {
		t.Fatal(err)
	}
	if q, err = s.Answer(ctx, q.ID, alice.ID, "draft"); err != nil || q.Revealed() {
		t.Fatalf("first answer: %+v, %v", q, err)
	}
	if q, err = s.Answer(ctx, q.ID, alice.ID, "Tuesday"); err != nil || len(q.Answers) != 1 || q.Answers[0].Text != "Tuesday" {
		t.Fatalf("changed answer: %+v, %v", q, err)
	}
	if q, err = s.Answer(ctx, q.ID, bob.ID, "Saturday"); err != nil || !q.Revealed() || len(q.Answers) != 2 {
		t.Fatalf("second answer must reveal: %+v, %v", q, err)
	}
	if _, err := s.Answer(ctx, q.ID, alice.ID, "too late"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("after reveal: want ErrConflict, got %v", err)
	}
	if _, err := s.GetQuestion(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown question: want ErrNotFound, got %v", err)
	}

	list, err := s.ListQuestions(ctx, m.Household.ID, 10)
	if err != nil || len(list) != 2 || list[0].ID != q.ID || len(list[0].Answers) != 2 || list[1].ID != solo.ID {
		t.Fatalf("list newest first with answers: %+v, %v", list, err)
	}
	if list, _ := s.ListQuestions(ctx, m.Household.ID, 1); len(list) != 1 {
		t.Fatalf("limit: got %d", len(list))
	}

	race, err := s.CreateQuestion(ctx, m.Household.ID, alice.ID, "Together?")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, u := range []domain.User{alice, bob} {
		wg.Go(func() {
			if _, err := s.Answer(ctx, race.ID, u.ID, "now"); err != nil {
				t.Errorf("concurrent answer: %v", err)
			}
		})
	}
	wg.Wait()
	if q, err := s.GetQuestion(ctx, race.ID); err != nil || !q.Revealed() {
		t.Fatalf("simultaneous answers must reveal: %+v, %v", q, err)
	}

	// Rewording: fine while only the asker has answered, refused once someone else has.
	edit, err := s.CreateQuestion(ctx, m.Household.ID, alice.ID, "Frist try?")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Answer(ctx, edit.ID, alice.ID, "mine"); err != nil {
		t.Fatal(err)
	}
	if q, err := s.RewordQuestion(ctx, edit.ID, "First try?"); err != nil || q.Text != "First try?" || len(q.Answers) != 1 {
		t.Fatalf("reword: %+v, %v", q, err)
	}
	if _, err := s.Answer(ctx, edit.ID, bob.ID, "his"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RewordQuestion(ctx, edit.ID, "Changed"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("reword after another answer: want ErrConflict, got %v", err)
	}
	if _, err := s.RewordQuestion(ctx, uuid.New(), "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("reword unknown: want ErrNotFound, got %v", err)
	}

	if err := s.DeleteQuestion(ctx, edit.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetQuestion(ctx, edit.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted question: want ErrNotFound, got %v", err)
	}
	if err := s.DeleteQuestion(ctx, edit.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete twice: want ErrNotFound, got %v", err)
	}
}

func join(t *testing.T, s Store, householdID uuid.UUID, inviter, joiner domain.User) {
	t.Helper()
	code := uuid.NewString()
	if err := s.CreateInvite(context.Background(), householdID, inviter.ID, hash(code), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptInvite(context.Background(), hash(code), joiner.ID); err != nil {
		t.Fatal(err)
	}
}

// Avatars: a picture a user sets survives signing in again, members of one household share,
// strangers don't, and removing it leaves authentik's picture.
func Avatars(t *testing.T, s Store) {
	ctx := context.Background()
	aliceSub := "sub-" + uuid.NewString()
	alice, err := s.UpsertByIdentity(ctx, domain.Identity{Subject: aliceSub, Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	bob, eve := user(t, s, "Bob"), user(t, s, "Eve")
	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	code := uuid.NewString()
	if err := s.CreateInvite(ctx, m.Household.ID, alice.ID, hash(code), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptInvite(ctx, hash(code), bob.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.SharesHousehold(ctx, bob.ID, alice.ID); err != nil || !ok {
		t.Fatalf("bob and alice: %v %v", ok, err)
	}
	if ok, err := s.SharesHousehold(ctx, eve.ID, alice.ID); err != nil || ok {
		t.Fatalf("eve and alice: %v %v", ok, err)
	}

	if _, err := s.Avatar(ctx, alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no avatar yet: want ErrNotFound, got %v", err)
	}
	u, err := s.SetAvatar(ctx, alice.ID, domain.Photo{ContentType: "image/jpeg", Data: []byte{1, 2, 3}})
	if err != nil || u.AvatarAt == nil || u.ID != alice.ID {
		t.Fatalf("set avatar: %+v, %v", u, err)
	}
	if p, err := s.Avatar(ctx, alice.ID); err != nil || len(p.Data) != 3 || p.ContentType != "image/jpeg" {
		t.Fatalf("avatar: %+v, %v", p, err)
	}
	again, err := s.UpsertByIdentity(ctx, domain.Identity{Subject: aliceSub, Name: "Alice", AvatarURL: "https://id.example.com/a.png"})
	if err != nil || again.AvatarAt == nil || again.AvatarURL != "https://id.example.com/a.png" {
		t.Fatalf("signing in again: %+v, %v", again, err)
	}
	members, _ := s.Members(ctx, m.Household.ID)
	if len(members) != 2 || members[0].User.AvatarAt == nil {
		t.Fatalf("members without alice's avatar: %+v", members)
	}
	u, err = s.DeleteAvatar(ctx, alice.ID)
	if err != nil || u.AvatarAt != nil {
		t.Fatalf("delete avatar: %+v, %v", u, err)
	}
	if _, err := s.Avatar(ctx, alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted avatar: want ErrNotFound, got %v", err)
	}
}
