package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MaxTripName    = 80
	MaxTripText    = 1000
	MaxOptionTitle = 100
	MaxOptionURL   = 500
	MaxPackingText = 100
	// MaxTrips, MaxTripOptions, MaxOptionPhotos and MaxPackingItems keep one household's lists small.
	MaxTrips        = 50
	MaxTripOptions  = 20
	MaxOptionPhotos = 12
	MaxPackingItems = 200
)

var (
	// ErrTripDates means an end before the start.
	ErrTripDates = fmt.Errorf("%w: a trip can't end before it starts", ErrInvalid)
	// ErrTripOption means a pick that isn't one of the trip's options.
	ErrTripOption = fmt.Errorf("%w: pick one of the trip's options", ErrInvalid)
	// ErrTripFull means a list that has reached its limit.
	ErrTripFull = fmt.Errorf("%w: that list is full", ErrConflict)
	// ErrTripPhoto means a photo that isn't a JPEG of at most MaxPhotoBytes.
	ErrTripPhoto = fmt.Errorf("%w: a photo must be a JPEG of at most %d MB", ErrInvalid, MaxPhotoBytes>>20)
	// ErrTripURL means a link that isn't an http(s) address.
	ErrTripURL = fmt.Errorf("%w: a link must start with http:// or https://", ErrInvalid)
)

// Trip is a trip or holiday the household plans together: where to, when, the places it is
// weighing up, and what to pack. Every member may change all of it.
type Trip struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	TripFields
	Options []TripOption
	Packing []PackingItem
	// Votes is each member's favourite option; a member without one isn't listed.
	Votes     []TripVote
	CreatedBy *uuid.UUID
	CreatedAt time.Time
}

// TripFields are what a member chooses for a trip.
type TripFields struct {
	Name        string
	Destination string
	// StartOn and EndOn are dates as midnight UTC; either may be unknown.
	StartOn *time.Time
	EndOn   *time.Time
	Notes   string
	// ChosenOptionID is the option the household decided on.
	ChosenOptionID *uuid.UUID
}

// TripOption is a place to stay or a plan the household is weighing up, with photos.
type TripOption struct {
	ID     uuid.UUID
	TripID uuid.UUID
	TripOptionFields
	Photos    []TripPhoto
	AddedBy   *uuid.UUID
	CreatedAt time.Time
}

// TripOptionFields are what a member chooses for an option.
type TripOptionFields struct {
	Title string
	// URL is a link to the listing, optional.
	URL   string
	Notes string
}

// TripPhoto is one picture of an option, kept in the PhotoStore; the first is its cover.
type TripPhoto struct {
	ID        uuid.UUID
	OptionID  uuid.UUID
	CreatedAt time.Time
}

// TripVote is a member's favourite option of a trip.
type TripVote struct {
	UserID   uuid.UUID
	OptionID uuid.UUID
}

// PackingItem is one thing to pack for a trip.
type PackingItem struct {
	ID        uuid.UUID
	TripID    uuid.UUID
	Text      string
	Done      bool
	CreatedAt time.Time
}

// PackingFields are what a member chooses for a packing item.
type PackingFields struct {
	Text string
	Done bool
}
