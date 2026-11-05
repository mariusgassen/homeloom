package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestWishesRankByHearts(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, bob, eve := newUser(t, store, "alice"), newUser(t, store, "bob"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hh := m.Household.ID
	store.AddMember(hh, bob.ID, domain.RoleMember)
	wishes := service.NewWishes(store, store)

	price := int64(129900)
	sofa, err := wishes.Create(ctx, alice, hh, domain.WishFields{Title: "  Sofa ", URL: " https://example.com/sofa ", PriceCents: &price})
	if err != nil || sofa.Title != "Sofa" || sofa.URL != "https://example.com/sofa" || sofa.CreatedBy == nil {
		t.Fatalf("create: %+v, %v", sofa, err)
	}
	lamp, _ := wishes.Create(ctx, bob, hh, domain.WishFields{Title: "Lamp"})
	rug, _ := wishes.Create(ctx, bob, hh, domain.WishFields{Title: "Rug"})

	zero, big := int64(0), int64(domain.MaxWishPriceCents+1)
	for name, f := range map[string]domain.WishFields{
		"no title":   {},
		"long note":  {Title: "x", Note: string(make([]rune, domain.MaxWishNote+1))},
		"not a link": {Title: "x", URL: "javascript:alert(1)"},
		"no host":    {Title: "x", URL: "https://"},
		"free":       {Title: "x", PriceCents: &zero},
		"too pricey": {Title: "x", PriceCents: &big},
	} {
		if _, err := wishes.Create(ctx, alice, hh, f); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := wishes.Create(ctx, eve, hh, domain.WishFields{Title: "x"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider adds: %v", err)
	}

	// Newest first while nobody has voted; then by hearts.
	list, _ := wishes.List(ctx, bob, hh)
	if len(list) != 3 || list[0].ID != rug.ID {
		t.Fatalf("unvoted order: %+v", list)
	}
	if _, err := wishes.Vote(ctx, alice, hh, lamp.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := wishes.Vote(ctx, bob, hh, lamp.ID, 1); err != nil {
		t.Fatal(err)
	}
	if w, err := wishes.Vote(ctx, bob, hh, sofa.ID, 3); err != nil || w.Hearts() != 3 {
		t.Fatalf("vote: %+v, %v", w, err)
	}
	// A second vote replaces the first.
	if w, _ := wishes.Vote(ctx, alice, hh, lamp.ID, 3); w.Hearts() != 4 || len(w.Votes) != 2 {
		t.Fatalf("revote: %+v", w)
	}
	for _, h := range []int{-1, 4} {
		if _, err := wishes.Vote(ctx, alice, hh, lamp.ID, h); !errors.Is(err, domain.ErrWishHearts) {
			t.Fatalf("hearts %d: %v", h, err)
		}
	}
	list, _ = wishes.List(ctx, alice, hh)
	if list[0].ID != lamp.ID || list[1].ID != sofa.ID || list[2].ID != rug.ID {
		t.Fatalf("ranked: %v %v %v", list[0].Title, list[1].Title, list[2].Title)
	}

	// Taking hearts back; a wish that came true goes last, whatever its hearts.
	if w, _ := wishes.Vote(ctx, alice, hh, lamp.ID, 0); w.Hearts() != 1 {
		t.Fatalf("unvote: %+v", w)
	}
	if w, err := wishes.Got(ctx, alice, hh, sofa.ID, true); err != nil || w.GotAt == nil {
		t.Fatalf("got: %+v, %v", w, err)
	}
	list, _ = wishes.List(ctx, alice, hh)
	if list[0].ID != lamp.ID || list[2].ID != sofa.ID {
		t.Fatalf("got goes last: %v %v %v", list[0].Title, list[1].Title, list[2].Title)
	}
	if w, _ := wishes.Got(ctx, alice, hh, sofa.ID, false); w.GotAt != nil {
		t.Fatalf("a wish again: %+v", w)
	}

	// Either member changes it; hearts stay.
	if w, err := wishes.Update(ctx, alice, hh, lamp.ID, domain.WishFields{Title: "Floor lamp"}); err != nil || w.Title != "Floor lamp" || w.Hearts() != 1 {
		t.Fatalf("update: %+v, %v", w, err)
	}
	if _, err := wishes.Update(ctx, eve, hh, lamp.ID, domain.WishFields{Title: "Mine"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider changes: %v", err)
	}
	if _, err := wishes.Vote(ctx, eve, hh, lamp.ID, 3); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider votes: %v", err)
	}
	if err := wishes.Delete(ctx, bob, hh, rug.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ = wishes.List(ctx, alice, hh); len(list) != 2 {
		t.Fatalf("after delete: %+v", list)
	}
}

func TestWishesKeepPhotos(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, eve := newUser(t, store, "alice"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hh := m.Household.ID
	w, err := service.NewWishes(store, store).Create(ctx, alice, hh, domain.WishFields{Title: "Plant"})
	if err != nil {
		t.Fatal(err)
	}
	jpeg := domain.Photo{ContentType: "image/jpeg", Data: []byte{0xFF, 0xD8, 0xFF, 1}}

	if _, err := service.NewWishes(store, store).SetPhoto(ctx, alice, hh, w.ID, jpeg); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("without a store: %v", err)
	}
	photos := memory.NewPhotos()
	wishes := service.NewWishes(store, store).WithPhotos(photos)
	if _, err := wishes.SetPhoto(ctx, alice, hh, w.ID, domain.Photo{ContentType: "image/jpeg", Data: []byte("<svg>")}); !errors.Is(err, domain.ErrWishPhoto) {
		t.Fatalf("not a JPEG: %v", err)
	}
	if _, err := wishes.SetPhoto(ctx, eve, hh, w.ID, jpeg); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}
	if _, err := wishes.Photo(ctx, alice, hh, w.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no photo yet: %v", err)
	}
	if got, err := wishes.SetPhoto(ctx, alice, hh, w.ID, jpeg); err != nil || got.PhotoAt == nil {
		t.Fatalf("set: %+v, %v", got, err)
	}
	if p, err := wishes.Photo(ctx, alice, hh, w.ID); err != nil || len(p.Data) != 4 {
		t.Fatalf("read: %v", err)
	}
	if got, err := wishes.DeletePhoto(ctx, alice, hh, w.ID); err != nil || got.PhotoAt != nil {
		t.Fatalf("delete photo: %+v, %v", got, err)
	}
	if _, err := photos.GetPhoto(ctx, "wishes/"+w.ID.String()+".jpg"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("photo left behind: %v", err)
	}
	// Deleting a wish takes its photo along.
	if _, err := wishes.SetPhoto(ctx, alice, hh, w.ID, jpeg); err != nil {
		t.Fatal(err)
	}
	if err := wishes.Delete(ctx, alice, hh, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := photos.GetPhoto(ctx, "wishes/"+w.ID.String()+".jpg"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("photo left behind after delete: %v", err)
	}
}
