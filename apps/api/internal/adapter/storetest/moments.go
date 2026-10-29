package storetest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Moments: others' private moments stay out, newest day first within a month, one gratitude line
// per person and day, this day in earlier years, photos, and each daily job claimed once.
func Moments(t *testing.T, s interface {
	Store
	port.MomentRepository
	port.JobRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Household.ID
	join(t, s, h, alice, bob)
	day := func(d string) time.Time {
		t.Helper()
		v, err := time.Parse(time.DateOnly, d)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	add := func(author domain.User, kind domain.MomentKind, d, text string, v domain.Visibility) domain.Moment {
		t.Helper()
		m := domain.Moment{HouseholdID: h, AuthorID: author.ID, Kind: kind, MomentFields: domain.MomentFields{Day: day(d), Visibility: v, Text: text}}
		var err error
		if kind == domain.KindGratitude {
			m, err = s.SaveGratitude(ctx, m)
		} else {
			m, err = s.CreateMoment(ctx, m)
		}
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	lake := add(alice, domain.KindMoment, "2026-08-02", "Lake", domain.VisibilityShared)
	add(alice, domain.KindMoment, "2025-10-28", "Apartment", domain.VisibilityShared)
	add(bob, domain.KindMoment, "2024-10-28", "Secret", domain.VisibilityPrivate)
	first := add(alice, domain.KindGratitude, "2026-10-27", "Coffee", domain.VisibilityShared)
	second := add(alice, domain.KindGratitude, "2026-10-27", "The walk", domain.VisibilityPrivate)
	add(bob, domain.KindMoment, "2026-10-01", "Autumn", domain.VisibilityShared)
	if second.ID != first.ID || second.Text != "The walk" || second.Visibility != domain.VisibilityPrivate {
		t.Fatalf("a second line on the same day replaces the first: %+v, %+v", first, second)
	}
	if got, err := s.GetMoment(ctx, lake.ID); err != nil || got.Text != "Lake" || !got.Day.Equal(day("2026-08-02")) || got.Kind != domain.KindMoment {
		t.Fatalf("get: %+v, %v", got, err)
	}
	if _, err := s.GetMoment(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown moment: want ErrNotFound, got %v", err)
	}

	texts := func(ms []domain.Moment, err error) []string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(ms))
		for i, m := range ms {
			out[i] = m.Text
		}
		return out
	}
	october := func(viewer domain.User) []string {
		return texts(s.ListMoments(ctx, h, viewer.ID, day("2026-10-01"), day("2026-11-01"), 10))
	}
	if got := october(alice); len(got) != 2 || got[0] != "The walk" || got[1] != "Autumn" {
		t.Fatalf("october for alice: %v", got)
	}
	if got := october(bob); len(got) != 1 || got[0] != "Autumn" {
		t.Fatalf("october for bob, without alice's private line: %v", got)
	}
	if d, err := s.LatestMomentDay(ctx, h, alice.ID, day("2026-10-01")); err != nil || !d.Equal(day("2026-08-02")) {
		t.Fatalf("latest before october: %v, %v", d, err)
	}
	if _, err := s.LatestMomentDay(ctx, h, alice.ID, day("2025-10-28")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("nothing before: want ErrNotFound, got %v", err)
	}
	if got := texts(s.MomentsOnDay(ctx, h, alice.ID, day("2026-10-28"), 10)); len(got) != 1 || got[0] != "Apartment" {
		t.Fatalf("on this day for alice: %v", got)
	}
	if got := texts(s.MomentsOnDay(ctx, h, bob.ID, day("2026-10-28"), 10)); len(got) != 2 || got[0] != "Apartment" || got[1] != "Secret" {
		t.Fatalf("on this day for bob: %v", got)
	}
	if line, err := s.RandomGratitude(ctx, h, alice.ID, day("2026-10-28")); err != nil || line.ID != first.ID {
		t.Fatalf("random line: %+v, %v", line, err)
	}
	if _, err := s.RandomGratitude(ctx, h, bob.ID, day("2026-10-28")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("alice's line is private: want ErrNotFound, got %v", err)
	}

	lake.Text, lake.Visibility = "Lake day", domain.VisibilityPrivate
	if got, err := s.UpdateMoment(ctx, lake); err != nil || got.Text != "Lake day" || got.Visibility != domain.VisibilityPrivate {
		t.Fatalf("update: %+v, %v", got, err)
	}
	if _, err := s.MomentPhoto(ctx, lake.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no photo yet: want ErrNotFound, got %v", err)
	}
	if err := s.SetMomentPhoto(ctx, lake.ID, domain.Photo{ContentType: "image/jpeg", Data: []byte{1, 2, 3}}); err != nil {
		t.Fatal(err)
	}
	if p, err := s.MomentPhoto(ctx, lake.ID); err != nil || len(p.Data) != 3 || p.ContentType != "image/jpeg" {
		t.Fatalf("photo: %+v, %v", p, err)
	}
	if got, _ := s.GetMoment(ctx, lake.ID); got.PhotoAt == nil {
		t.Fatalf("photoAt not set: %+v", got)
	}
	if err := s.DeleteMomentPhoto(ctx, lake.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetMoment(ctx, lake.ID); got.PhotoAt != nil {
		t.Fatalf("photoAt kept: %+v", got)
	}
	if err := s.SetMomentPhoto(ctx, lake.ID, domain.Photo{ContentType: "image/jpeg", Data: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkMomentPhoto(ctx, lake.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetMoment(ctx, lake.ID); got.PhotoAt == nil {
		t.Fatalf("photoAt not set by mark: %+v", got)
	}
	if _, err := s.MomentPhoto(ctx, lake.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("marked photo kept its old copy: %v", err)
	}
	if err := s.SetMomentPhoto(ctx, lake.ID, domain.Photo{ContentType: "image/jpeg", Data: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMoment(ctx, lake.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MomentPhoto(ctx, lake.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("photo outlived its moment: %v", err)
	}

	ids, err := s.HouseholdIDs(ctx)
	if err != nil || !slices.Contains(ids, h) {
		t.Fatalf("household ids: %v, %v", ids, err)
	}
	job := "test-" + uuid.NewString()
	if ok, err := s.ClaimJobRun(ctx, job, day("2026-10-28")); err != nil || !ok {
		t.Fatalf("first claim: %v, %v", ok, err)
	}
	if ok, err := s.ClaimJobRun(ctx, job, day("2026-10-28")); err != nil || ok {
		t.Fatalf("second claim: %v, %v", ok, err)
	}
	if ok, err := s.ClaimJobRun(ctx, job, day("2026-10-29")); err != nil || !ok {
		t.Fatalf("next day: %v, %v", ok, err)
	}
}
