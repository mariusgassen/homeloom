package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestOnlyTheOwnerKeepsMembersDetails(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, bob, eve := newUser(t, store, "alice"), newUser(t, store, "bob"), newUser(t, store, "eve")
	households := service.NewHousehold(store).WithBirthdays(store)
	m, _ := households.Create(ctx, alice, "Home")
	hh := m.Household.ID
	store.AddMember(hh, bob.ID, domain.RoleMember)
	members := service.NewMembers(store, store, store)

	b := domain.Birthday{Month: time.March, Day: 14, Year: 1990}
	if err := members.SetBirthday(ctx, alice, hh, bob.ID, b); err != nil {
		t.Fatalf("owner sets: %v", err)
	}
	d, _ := households.Get(ctx, bob, hh)
	for _, mm := range d.Members {
		if got := mm.Birthday != nil; got != (mm.User.ID == bob.ID) {
			t.Errorf("%s: birthday present = %v", mm.User.Name, got)
		}
	}
	if err := members.SetBirthday(ctx, alice, hh, bob.ID, domain.Birthday{Month: time.February, Day: 31}); !errors.Is(err, domain.ErrBirthday) {
		t.Errorf("not a date: %v", err)
	}
	if err := members.SetBirthday(ctx, bob, hh, alice.ID, b); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("member: %v", err)
	}
	if err := members.SetBirthday(ctx, eve, hh, bob.ID, b); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("outsider: %v", err)
	}
	if err := members.SetBirthday(ctx, alice, hh, eve.ID, b); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("not a member: %v", err)
	}
	if err := members.ClearBirthday(ctx, alice, hh, bob.ID); err != nil {
		t.Errorf("clear: %v", err)
	}
	if _, err := members.SetAvatar(ctx, alice, hh, bob.ID, domain.Photo{ContentType: "image/jpeg", Data: []byte("nope")}); !errors.Is(err, domain.ErrAvatar) {
		t.Errorf("not a jpeg: %v", err)
	}
	jpeg := domain.Photo{ContentType: "image/jpeg", Data: []byte{0xFF, 0xD8, 0xFF, 0xE0}}
	if u, err := members.SetAvatar(ctx, alice, hh, bob.ID, jpeg); err != nil || u.AvatarAt == nil {
		t.Errorf("owner sets picture: %+v, %v", u, err)
	}
	if _, err := members.DeleteAvatar(ctx, bob, hh, alice.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("member removes: %v", err)
	}
}
