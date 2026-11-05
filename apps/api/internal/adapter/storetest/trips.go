package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Trips checks a TripRepository: trips are kept per household, with their options, photos, votes
// and packing items, and removing an option takes its photos and votes along.
func Trips(t *testing.T, s interface {
	Store
	port.TripRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	home, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, "Elsewhere", bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	join(t, s, home.Household.ID, alice, bob)
	hh := home.Household.ID
	day := func(m time.Month, d int) *time.Time {
		v := time.Date(2027, m, d, 0, 0, 0, 0, time.UTC)
		return &v
	}

	later, err := s.CreateTrip(ctx, domain.Trip{HouseholdID: hh, CreatedBy: &alice.ID, TripFields: domain.TripFields{Name: "Autumn", StartOn: day(10, 1)}})
	if err != nil || later.ID == uuid.Nil || later.Name != "Autumn" || later.Options == nil {
		t.Fatalf("create: %+v, %v", later, err)
	}
	undated, _ := s.CreateTrip(ctx, domain.Trip{HouseholdID: hh, TripFields: domain.TripFields{Name: "Someday"}})
	summer, _ := s.CreateTrip(ctx, domain.Trip{HouseholdID: hh, CreatedBy: &bob.ID, TripFields: domain.TripFields{
		Name: "Summer", Destination: "Italy", StartOn: day(7, 1), EndOn: day(7, 14), Notes: "Sun",
	}})
	if _, err := s.CreateTrip(ctx, domain.Trip{HouseholdID: other.Household.ID, TripFields: domain.TripFields{Name: "Theirs"}}); err != nil {
		t.Fatal(err)
	}
	trips, err := s.Trips(ctx, hh, 10)
	if err != nil || len(trips) != 3 || trips[0].ID != summer.ID || trips[1].ID != later.ID || trips[2].ID != undated.ID {
		t.Fatalf("list: soonest first, undated last, only this household's: %+v, %v", trips, err)
	}
	if _, err := s.Trip(ctx, other.Household.ID, summer.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get from another household: want ErrNotFound, got %v", err)
	}

	villa, err := s.CreateTripOption(ctx, domain.TripOption{TripID: summer.ID, AddedBy: &alice.ID, TripOptionFields: domain.TripOptionFields{Title: "Villa", URL: "https://example.com/v", Notes: "Pool"}})
	if err != nil || villa.ID == uuid.Nil {
		t.Fatalf("create option: %+v, %v", villa, err)
	}
	flat, _ := s.CreateTripOption(ctx, domain.TripOption{TripID: summer.ID, TripOptionFields: domain.TripOptionFields{Title: "Flat"}})
	p1, err := s.CreateTripPhoto(ctx, domain.TripPhoto{OptionID: villa.ID})
	if err != nil || p1.ID == uuid.Nil {
		t.Fatalf("create photo: %+v, %v", p1, err)
	}
	p2, _ := s.CreateTripPhoto(ctx, domain.TripPhoto{OptionID: villa.ID})
	if err := s.SetTripVote(ctx, summer.ID, alice.ID, &villa.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTripVote(ctx, summer.ID, bob.ID, &villa.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTripVote(ctx, summer.ID, bob.ID, &flat.ID); err != nil {
		t.Fatal(err)
	}
	pack, err := s.CreatePackingItem(ctx, summer.ID, "Sunscreen")
	if err != nil || pack.ID == uuid.Nil || pack.Done {
		t.Fatalf("create packing item: %+v, %v", pack, err)
	}
	if err := s.UpdatePackingItem(ctx, summer.ID, pack.ID, domain.PackingFields{Text: "Sun cream", Done: true}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Trip(ctx, hh, summer.ID)
	if err != nil || len(got.Options) != 2 || got.Options[0].ID != villa.ID || got.Options[0].URL != "https://example.com/v" ||
		len(got.Options[0].Photos) != 2 || got.Options[0].Photos[0].ID != p1.ID || got.Options[0].Photos[1].ID != p2.ID || len(got.Options[1].Photos) != 0 {
		t.Fatalf("trip with options and photos: %+v, %v", got, err)
	}
	if len(got.Votes) != 2 {
		t.Fatalf("one vote each: %+v", got.Votes)
	}
	for _, v := range got.Votes {
		if (v.UserID == alice.ID) != (v.OptionID == villa.ID) {
			t.Fatalf("voting again moves the vote: %+v", v)
		}
	}
	if len(got.Packing) != 1 || got.Packing[0].Text != "Sun cream" || !got.Packing[0].Done {
		t.Fatalf("packing: %+v", got.Packing)
	}
	if all, _ := s.Trips(ctx, hh, 10); len(all[0].Options) != 2 || len(all[0].Options[0].Photos) != 2 || len(all[0].Votes) != 2 || len(all[0].Packing) != 1 {
		t.Fatalf("list carries the parts: %+v", all[0])
	}

	// Update the trip and choose an option.
	got.Destination, got.ChosenOptionID, got.EndOn = "Rome", &villa.ID, nil
	if upd, err := s.UpdateTrip(ctx, got); err != nil || upd.Destination != "Rome" || upd.ChosenOptionID == nil || *upd.ChosenOptionID != villa.ID || upd.EndOn != nil {
		t.Fatalf("update: %+v, %v", upd, err)
	}
	if err := s.UpdateTripOption(ctx, summer.ID, domain.TripOption{ID: flat.ID, TripOptionFields: domain.TripOptionFields{Title: "Cosy flat", Notes: "Centre"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTripOption(ctx, later.ID, domain.TripOption{ID: flat.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("option of another trip: want ErrNotFound, got %v", err)
	}
	if err := s.DeleteTripPhoto(ctx, villa.ID, p1.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTripPhoto(ctx, villa.ID, p1.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete photo twice: want ErrNotFound, got %v", err)
	}
	if err := s.SetTripVote(ctx, summer.ID, alice.ID, nil); err != nil {
		t.Fatal(err)
	}

	// Removing the chosen option un-chooses it and drops its votes and photos.
	if err := s.DeleteTripOption(ctx, summer.ID, villa.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Trip(ctx, hh, summer.ID)
	if got.ChosenOptionID != nil || len(got.Options) != 1 || got.Options[0].Title != "Cosy flat" || len(got.Votes) != 1 || got.Votes[0].OptionID != flat.ID {
		t.Fatalf("after removing the option: %+v", got)
	}
	if err := s.DeletePackingItem(ctx, summer.ID, pack.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePackingItem(ctx, summer.ID, pack.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete item twice: want ErrNotFound, got %v", err)
	}

	if err := s.DeleteTrip(ctx, other.Household.ID, summer.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete from another household: want ErrNotFound, got %v", err)
	}
	if err := s.DeleteTrip(ctx, hh, summer.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Trip(ctx, hh, summer.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete: want ErrNotFound, got %v", err)
	}
}
