package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Trips is the household's trips. Everything on a trip is shared, so every member may change it.
// Photos of options are kept in a PhotoStore; without one the rest still works.
type Trips struct {
	households port.HouseholdRepository
	trips      port.TripRepository
	photos     port.PhotoStore
}

func NewTrips(households port.HouseholdRepository, trips port.TripRepository) *Trips {
	return &Trips{households: households, trips: trips}
}

// WithPhotos lets options have photos, kept in photos.
func (s *Trips) WithPhotos(photos port.PhotoStore) *Trips {
	s.photos = photos
	return s
}

func (s *Trips) List(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Trip, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.trips.Trips(ctx, householdID, domain.MaxTrips)
}

func (s *Trips) Create(ctx context.Context, actor domain.User, householdID uuid.UUID, f domain.TripFields) (domain.Trip, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Trip{}, err
	}
	// A new trip has no options yet, so there is nothing to have chosen.
	f.ChosenOptionID = nil
	f, err := checkTrip(f)
	if err != nil {
		return domain.Trip{}, err
	}
	existing, err := s.trips.Trips(ctx, householdID, domain.MaxTrips+1)
	if err != nil {
		return domain.Trip{}, err
	}
	if len(existing) >= domain.MaxTrips {
		return domain.Trip{}, domain.ErrTripFull
	}
	return s.trips.CreateTrip(ctx, domain.Trip{HouseholdID: householdID, TripFields: f, CreatedBy: &actor.ID})
}

func (s *Trips) Update(ctx context.Context, actor domain.User, householdID, id uuid.UUID, f domain.TripFields) (domain.Trip, error) {
	t, err := s.load(ctx, actor, householdID, id)
	if err != nil {
		return domain.Trip{}, err
	}
	if f, err = checkTrip(f); err != nil {
		return domain.Trip{}, err
	}
	if f.ChosenOptionID != nil && !hasOption(t, *f.ChosenOptionID) {
		return domain.Trip{}, domain.ErrTripOption
	}
	t.TripFields = f
	if _, err := s.trips.UpdateTrip(ctx, t); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, id)
}

func (s *Trips) Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error {
	t, err := s.load(ctx, actor, householdID, id)
	if err != nil {
		return err
	}
	// The photos go first, so a deleted trip never leaves them behind.
	for _, o := range t.Options {
		if err := s.deletePhotos(ctx, o.Photos); err != nil {
			return err
		}
	}
	return s.trips.DeleteTrip(ctx, householdID, id)
}

func (s *Trips) AddOption(ctx context.Context, actor domain.User, householdID, tripID uuid.UUID, f domain.TripOptionFields) (domain.Trip, error) {
	t, err := s.load(ctx, actor, householdID, tripID)
	if err != nil {
		return domain.Trip{}, err
	}
	if f, err = checkOption(f); err != nil {
		return domain.Trip{}, err
	}
	if len(t.Options) >= domain.MaxTripOptions {
		return domain.Trip{}, domain.ErrTripFull
	}
	if _, err := s.trips.CreateTripOption(ctx, domain.TripOption{TripID: tripID, TripOptionFields: f, AddedBy: &actor.ID}); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, tripID)
}

func (s *Trips) UpdateOption(ctx context.Context, actor domain.User, householdID, tripID, optionID uuid.UUID, f domain.TripOptionFields) (domain.Trip, error) {
	t, err := s.load(ctx, actor, householdID, tripID)
	if err != nil {
		return domain.Trip{}, err
	}
	if !hasOption(t, optionID) {
		return domain.Trip{}, domain.ErrNotFound
	}
	if f, err = checkOption(f); err != nil {
		return domain.Trip{}, err
	}
	if err := s.trips.UpdateTripOption(ctx, tripID, domain.TripOption{ID: optionID, TripOptionFields: f}); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, tripID)
}

func (s *Trips) DeleteOption(ctx context.Context, actor domain.User, householdID, tripID, optionID uuid.UUID) (domain.Trip, error) {
	t, err := s.load(ctx, actor, householdID, tripID)
	if err != nil {
		return domain.Trip{}, err
	}
	i := optionIndex(t, optionID)
	if i < 0 {
		return domain.Trip{}, domain.ErrNotFound
	}
	if err := s.deletePhotos(ctx, t.Options[i].Photos); err != nil {
		return domain.Trip{}, err
	}
	if err := s.trips.DeleteTripOption(ctx, tripID, optionID); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, tripID)
}

func (s *Trips) Vote(ctx context.Context, actor domain.User, householdID, tripID, optionID uuid.UUID) (domain.Trip, error) {
	t, err := s.load(ctx, actor, householdID, tripID)
	if err != nil {
		return domain.Trip{}, err
	}
	if !hasOption(t, optionID) {
		return domain.Trip{}, domain.ErrNotFound
	}
	if err := s.trips.SetTripVote(ctx, tripID, actor.ID, &optionID); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, tripID)
}

func (s *Trips) Unvote(ctx context.Context, actor domain.User, householdID, tripID uuid.UUID) (domain.Trip, error) {
	if _, err := s.load(ctx, actor, householdID, tripID); err != nil {
		return domain.Trip{}, err
	}
	if err := s.trips.SetTripVote(ctx, tripID, actor.ID, nil); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, tripID)
}

func (s *Trips) AddPhoto(ctx context.Context, actor domain.User, householdID, tripID, optionID uuid.UUID, p domain.Photo) (domain.Trip, error) {
	t, err := s.load(ctx, actor, householdID, tripID)
	if err != nil {
		return domain.Trip{}, err
	}
	i := optionIndex(t, optionID)
	if i < 0 {
		return domain.Trip{}, domain.ErrNotFound
	}
	if s.photos == nil {
		return domain.Trip{}, domain.ErrUnavailable
	}
	// The app sends JPEGs; checking the bytes as well keeps anything else from being served as one.
	if p.ContentType != "image/jpeg" || len(p.Data) > domain.MaxPhotoBytes || !bytes.HasPrefix(p.Data, []byte{0xFF, 0xD8, 0xFF}) {
		return domain.Trip{}, domain.ErrTripPhoto
	}
	if len(t.Options[i].Photos) >= domain.MaxOptionPhotos {
		return domain.Trip{}, domain.ErrTripFull
	}
	ph, err := s.trips.CreateTripPhoto(ctx, domain.TripPhoto{OptionID: optionID})
	if err != nil {
		return domain.Trip{}, err
	}
	if err := s.photos.PutPhoto(ctx, tripPhotoKey(ph.ID), p); err != nil {
		// Nothing should point at a photo that isn't there.
		_ = s.trips.DeleteTripPhoto(ctx, optionID, ph.ID)
		return domain.Trip{}, photosDown(err)
	}
	return s.trips.Trip(ctx, householdID, tripID)
}

func (s *Trips) Photo(ctx context.Context, actor domain.User, householdID, tripID, photoID uuid.UUID) (domain.Photo, error) {
	t, err := s.load(ctx, actor, householdID, tripID)
	if err != nil {
		return domain.Photo{}, err
	}
	if _, _, ok := findPhoto(t, photoID); !ok {
		return domain.Photo{}, domain.ErrNotFound
	}
	if s.photos == nil {
		return domain.Photo{}, domain.ErrUnavailable
	}
	p, err := s.photos.GetPhoto(ctx, tripPhotoKey(photoID))
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.Photo{}, photosDown(err)
	}
	return p, err
}

func (s *Trips) DeletePhoto(ctx context.Context, actor domain.User, householdID, tripID, photoID uuid.UUID) (domain.Trip, error) {
	t, err := s.load(ctx, actor, householdID, tripID)
	if err != nil {
		return domain.Trip{}, err
	}
	optionID, _, ok := findPhoto(t, photoID)
	if !ok {
		return domain.Trip{}, domain.ErrNotFound
	}
	if s.photos == nil {
		return domain.Trip{}, domain.ErrUnavailable
	}
	if err := s.photos.DeletePhoto(ctx, tripPhotoKey(photoID)); err != nil {
		return domain.Trip{}, photosDown(err)
	}
	if err := s.trips.DeleteTripPhoto(ctx, optionID, photoID); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, tripID)
}

func (s *Trips) AddPacking(ctx context.Context, actor domain.User, householdID, tripID uuid.UUID, text string) (domain.Trip, error) {
	t, err := s.load(ctx, actor, householdID, tripID)
	if err != nil {
		return domain.Trip{}, err
	}
	if text, err = checkPackingText(text); err != nil {
		return domain.Trip{}, err
	}
	if len(t.Packing) >= domain.MaxPackingItems {
		return domain.Trip{}, domain.ErrTripFull
	}
	if _, err := s.trips.CreatePackingItem(ctx, tripID, text); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, tripID)
}

func (s *Trips) UpdatePacking(ctx context.Context, actor domain.User, householdID, tripID, itemID uuid.UUID, f domain.PackingFields) (domain.Trip, error) {
	if _, err := s.load(ctx, actor, householdID, tripID); err != nil {
		return domain.Trip{}, err
	}
	var err error
	if f.Text, err = checkPackingText(f.Text); err != nil {
		return domain.Trip{}, err
	}
	if err := s.trips.UpdatePackingItem(ctx, tripID, itemID, f); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, tripID)
}

func (s *Trips) DeletePacking(ctx context.Context, actor domain.User, householdID, tripID, itemID uuid.UUID) (domain.Trip, error) {
	if _, err := s.load(ctx, actor, householdID, tripID); err != nil {
		return domain.Trip{}, err
	}
	if err := s.trips.DeletePackingItem(ctx, tripID, itemID); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, tripID)
}

// load returns a trip of a household the actor belongs to; anything else is domain.ErrNotFound.
func (s *Trips) load(ctx context.Context, actor domain.User, householdID, id uuid.UUID) (domain.Trip, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Trip{}, err
	}
	return s.trips.Trip(ctx, householdID, id)
}

// deletePhotos removes the photos' files; a store that is down stops it.
func (s *Trips) deletePhotos(ctx context.Context, photos []domain.TripPhoto) error {
	if len(photos) == 0 {
		return nil
	}
	if s.photos == nil {
		return domain.ErrUnavailable
	}
	for _, p := range photos {
		if err := s.photos.DeletePhoto(ctx, tripPhotoKey(p.ID)); err != nil {
			return photosDown(err)
		}
	}
	return nil
}

func hasOption(t domain.Trip, id uuid.UUID) bool { return optionIndex(t, id) >= 0 }

func optionIndex(t domain.Trip, id uuid.UUID) int {
	for i, o := range t.Options {
		if o.ID == id {
			return i
		}
	}
	return -1
}

// findPhoto returns the option and position the photo has on the trip.
func findPhoto(t domain.Trip, id uuid.UUID) (optionID uuid.UUID, pos int, ok bool) {
	for _, o := range t.Options {
		for i, p := range o.Photos {
			if p.ID == id {
				return o.ID, i, true
			}
		}
	}
	return uuid.Nil, 0, false
}

func checkTrip(f domain.TripFields) (domain.TripFields, error) {
	f.Name = strings.TrimSpace(f.Name)
	f.Destination = strings.TrimSpace(f.Destination)
	f.Notes = strings.TrimSpace(f.Notes)
	if f.Name == "" || utf8.RuneCountInString(f.Name) > domain.MaxTripName {
		return f, domain.TextError{Max: domain.MaxTripName}
	}
	if utf8.RuneCountInString(f.Destination) > domain.MaxTripName || utf8.RuneCountInString(f.Notes) > domain.MaxTripText {
		return f, domain.TextError{Max: domain.MaxTripText}
	}
	f.StartOn, f.EndOn = dayOf(f.StartOn), dayOf(f.EndOn)
	if f.StartOn != nil && f.EndOn != nil && f.EndOn.Before(*f.StartOn) {
		return f, domain.ErrTripDates
	}
	return f, nil
}

func checkOption(f domain.TripOptionFields) (domain.TripOptionFields, error) {
	f.Title = strings.TrimSpace(f.Title)
	f.URL = strings.TrimSpace(f.URL)
	f.Notes = strings.TrimSpace(f.Notes)
	if f.Title == "" || utf8.RuneCountInString(f.Title) > domain.MaxOptionTitle {
		return f, domain.TextError{Max: domain.MaxOptionTitle}
	}
	if utf8.RuneCountInString(f.Notes) > domain.MaxTripText {
		return f, domain.TextError{Max: domain.MaxTripText}
	}
	if f.URL != "" {
		u, err := url.Parse(f.URL)
		if len(f.URL) > domain.MaxOptionURL || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return f, domain.ErrTripURL
		}
	}
	return f, nil
}

func checkPackingText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > domain.MaxPackingText {
		return text, domain.TextError{Max: domain.MaxPackingText}
	}
	return text, nil
}

// dayOf cuts a time down to its date, as midnight UTC.
func dayOf(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return &d
}

// photosDown reports a failing photo store as unavailable, so photos stop working, not the trips.
func photosDown(err error) error {
	return fmt.Errorf("%w: trip photos: %w", domain.ErrUnavailable, err)
}

// tripPhotoKey is where a trip photo is kept in the PhotoStore.
func tripPhotoKey(id uuid.UUID) string { return "trips/" + id.String() + ".jpg" }
