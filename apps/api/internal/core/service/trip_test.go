package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

type tripFixture struct {
	store           *memory.Store
	photos          *memory.Photos
	trips           *service.Trips
	alice, bob, eve domain.User
	hh              uuid.UUID
}

func newTripFixture(t *testing.T) *tripFixture {
	t.Helper()
	f := &tripFixture{store: memory.NewStore(), photos: memory.NewPhotos()}
	f.alice, f.bob, f.eve = newUser(t, f.store, "alice"), newUser(t, f.store, "bob"), newUser(t, f.store, "eve")
	m, err := service.NewHousehold(f.store).Create(context.Background(), f.alice, "Home")
	if err != nil {
		t.Fatal(err)
	}
	f.hh = m.Household.ID
	f.store.AddMember(f.hh, f.bob.ID, domain.RoleMember)
	f.trips = service.NewTrips(f.store, f.store).WithPhotos(f.photos)
	return f
}

func tripDay(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

var jpeg = domain.Photo{ContentType: "image/jpeg", Data: []byte{0xFF, 0xD8, 0xFF, 1}}

func TestTripsPlanWithOptionsVotesAndAChoice(t *testing.T) {
	ctx := context.Background()
	f := newTripFixture(t)

	if _, err := f.trips.Create(ctx, f.alice, f.hh, domain.TripFields{Name: " "}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty name: %v", err)
	}
	if _, err := f.trips.Create(ctx, f.alice, f.hh, domain.TripFields{Name: "Italy", StartOn: tripDay(2027, 7, 10), EndOn: tripDay(2027, 7, 1)}); !errors.Is(err, domain.ErrTripDates) {
		t.Fatalf("end before start: %v", err)
	}
	if _, err := f.trips.Create(ctx, f.eve, f.hh, domain.TripFields{Name: "Italy"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}

	later, err := f.trips.Create(ctx, f.alice, f.hh, domain.TripFields{Name: "Autumn", StartOn: tripDay(2027, 10, 1)})
	if err != nil {
		t.Fatal(err)
	}
	undated, _ := f.trips.Create(ctx, f.alice, f.hh, domain.TripFields{Name: "Someday"})
	trip, err := f.trips.Create(ctx, f.bob, f.hh, domain.TripFields{Name: "Summer", Destination: "Italy", StartOn: tripDay(2027, 7, 1), EndOn: tripDay(2027, 7, 14)})
	if err != nil {
		t.Fatal(err)
	}
	list, err := f.trips.List(ctx, f.alice, f.hh)
	if err != nil || len(list) != 3 || list[0].ID != trip.ID || list[1].ID != later.ID || list[2].ID != undated.ID {
		t.Fatalf("list: %+v, %v", list, err)
	}
	if _, err := f.trips.List(ctx, f.eve, f.hh); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider list: %v", err)
	}

	if _, err := f.trips.AddOption(ctx, f.alice, f.hh, trip.ID, domain.TripOptionFields{Title: "Villa", URL: "javascript:alert(1)"}); !errors.Is(err, domain.ErrTripURL) {
		t.Fatalf("bad link: %v", err)
	}
	got, err := f.trips.AddOption(ctx, f.alice, f.hh, trip.ID, domain.TripOptionFields{Title: "Villa", URL: "https://example.com/villa"})
	if err != nil {
		t.Fatal(err)
	}
	got, err = f.trips.AddOption(ctx, f.bob, f.hh, trip.ID, domain.TripOptionFields{Title: "Flat"})
	if err != nil || len(got.Options) != 2 {
		t.Fatalf("options: %+v, %v", got.Options, err)
	}
	villa, flat := got.Options[0], got.Options[1]

	// Each member has one favourite; voting again moves it.
	if _, err := f.trips.Vote(ctx, f.alice, f.hh, trip.ID, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("vote for nothing: %v", err)
	}
	if _, err := f.trips.Vote(ctx, f.alice, f.hh, trip.ID, villa.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.trips.Vote(ctx, f.bob, f.hh, trip.ID, villa.ID); err != nil {
		t.Fatal(err)
	}
	got, err = f.trips.Vote(ctx, f.bob, f.hh, trip.ID, flat.ID)
	if err != nil || len(got.Votes) != 2 {
		t.Fatalf("votes: %+v, %v", got.Votes, err)
	}
	for _, v := range got.Votes {
		if (v.UserID == f.alice.ID) != (v.OptionID == villa.ID) {
			t.Fatalf("wrong vote: %+v", v)
		}
	}
	if got, _ = f.trips.Unvote(ctx, f.bob, f.hh, trip.ID); len(got.Votes) != 1 {
		t.Fatalf("unvote: %+v", got.Votes)
	}

	// Only one of its own options can be chosen; removing the chosen one un-chooses it.
	fields := trip.TripFields
	fields.ChosenOptionID = &villa.ID
	other, _ := f.trips.Create(ctx, f.alice, f.hh, domain.TripFields{Name: "Other"})
	if _, err := f.trips.Update(ctx, f.alice, f.hh, other.ID, fields); !errors.Is(err, domain.ErrTripOption) {
		t.Fatalf("option of another trip: %v", err)
	}
	if got, err = f.trips.Update(ctx, f.bob, f.hh, trip.ID, fields); err != nil || got.ChosenOptionID == nil || *got.ChosenOptionID != villa.ID {
		t.Fatalf("choose: %+v, %v", got.ChosenOptionID, err)
	}
	if got, err = f.trips.DeleteOption(ctx, f.alice, f.hh, trip.ID, villa.ID); err != nil || got.ChosenOptionID != nil || len(got.Options) != 1 || len(got.Votes) != 0 {
		t.Fatalf("delete option: %+v, %v", got, err)
	}
}

func TestTripOptionsKeepPhotos(t *testing.T) {
	ctx := context.Background()
	f := newTripFixture(t)
	trip, _ := f.trips.Create(ctx, f.alice, f.hh, domain.TripFields{Name: "Summer"})
	trip, _ = f.trips.AddOption(ctx, f.alice, f.hh, trip.ID, domain.TripOptionFields{Title: "Villa"})
	opt := trip.Options[0].ID

	if _, err := service.NewTrips(f.store, f.store).AddPhoto(ctx, f.alice, f.hh, trip.ID, opt, jpeg); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("without a store: %v", err)
	}
	if _, err := f.trips.AddPhoto(ctx, f.alice, f.hh, trip.ID, opt, domain.Photo{ContentType: "image/jpeg", Data: []byte("<svg>")}); !errors.Is(err, domain.ErrTripPhoto) {
		t.Fatalf("not a JPEG: %v", err)
	}
	if _, err := f.trips.AddPhoto(ctx, f.eve, f.hh, trip.ID, opt, jpeg); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}
	got, err := f.trips.AddPhoto(ctx, f.bob, f.hh, trip.ID, opt, jpeg)
	if err != nil || len(got.Options[0].Photos) != 1 {
		t.Fatalf("add: %+v, %v", got, err)
	}
	id := got.Options[0].Photos[0].ID
	if p, err := f.trips.Photo(ctx, f.alice, f.hh, trip.ID, id); err != nil || len(p.Data) != 4 {
		t.Fatalf("read: %v", err)
	}
	if _, err := f.trips.Photo(ctx, f.eve, f.hh, trip.ID, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider read: %v", err)
	}
	if got, err = f.trips.DeletePhoto(ctx, f.alice, f.hh, trip.ID, id); err != nil || len(got.Options[0].Photos) != 0 {
		t.Fatalf("delete: %+v, %v", got, err)
	}
	if _, err := f.photos.GetPhoto(ctx, "trips/"+id.String()+".jpg"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("photo left behind: %v", err)
	}

	// Removing an option or the trip takes its photos along.
	got, _ = f.trips.AddPhoto(ctx, f.alice, f.hh, trip.ID, opt, jpeg)
	a := got.Options[0].Photos[0].ID
	got, _ = f.trips.AddOption(ctx, f.alice, f.hh, trip.ID, domain.TripOptionFields{Title: "Flat"})
	got, _ = f.trips.AddPhoto(ctx, f.alice, f.hh, trip.ID, got.Options[1].ID, jpeg)
	b := got.Options[1].Photos[0].ID
	if _, err := f.trips.DeleteOption(ctx, f.alice, f.hh, trip.ID, opt); err != nil {
		t.Fatal(err)
	}
	if _, err := f.photos.GetPhoto(ctx, "trips/"+a.String()+".jpg"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("option left its photo: %v", err)
	}
	if err := f.trips.Delete(ctx, f.bob, f.hh, trip.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.photos.GetPhoto(ctx, "trips/"+b.String()+".jpg"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("trip left its photo: %v", err)
	}
	if list, _ := f.trips.List(ctx, f.alice, f.hh); len(list) != 0 {
		t.Fatalf("trip still listed: %+v", list)
	}
}

func TestTripPackingList(t *testing.T) {
	ctx := context.Background()
	f := newTripFixture(t)
	trip, _ := f.trips.Create(ctx, f.alice, f.hh, domain.TripFields{Name: "Summer"})

	if _, err := f.trips.AddPacking(ctx, f.alice, f.hh, trip.ID, "  "); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := f.trips.AddPacking(ctx, f.eve, f.hh, trip.ID, "Sunscreen"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}
	got, err := f.trips.AddPacking(ctx, f.alice, f.hh, trip.ID, " Sunscreen ")
	if err != nil || len(got.Packing) != 1 || got.Packing[0].Text != "Sunscreen" || got.Packing[0].Done {
		t.Fatalf("add: %+v, %v", got.Packing, err)
	}
	id := got.Packing[0].ID
	if got, err = f.trips.UpdatePacking(ctx, f.bob, f.hh, trip.ID, id, domain.PackingFields{Text: "Sunscreen", Done: true}); err != nil || !got.Packing[0].Done {
		t.Fatalf("tick: %+v, %v", got.Packing, err)
	}
	if _, err := f.trips.UpdatePacking(ctx, f.bob, f.hh, trip.ID, uuid.New(), domain.PackingFields{Text: "x"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown item: %v", err)
	}
	if got, err = f.trips.DeletePacking(ctx, f.alice, f.hh, trip.ID, id); err != nil || len(got.Packing) != 0 {
		t.Fatalf("delete: %+v, %v", got.Packing, err)
	}
}

func TestTripListsHaveLimits(t *testing.T) {
	ctx := context.Background()
	f := newTripFixture(t)
	trip, _ := f.trips.Create(ctx, f.alice, f.hh, domain.TripFields{Name: "Summer"})
	var err error
	for i := 0; i < domain.MaxPackingItems; i++ {
		if trip, err = f.trips.AddPacking(ctx, f.alice, f.hh, trip.ID, "item"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.trips.AddPacking(ctx, f.alice, f.hh, trip.ID, "one more"); !errors.Is(err, domain.ErrTripFull) {
		t.Fatalf("packing limit: %v", err)
	}
}
