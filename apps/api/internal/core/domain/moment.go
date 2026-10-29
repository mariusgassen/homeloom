package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MaxMomentText    = 2000
	MaxGratitudeText = 200
	// MaxPhotoBytes is the largest photo a moment can have; the app makes photos smaller first.
	MaxPhotoBytes = 5 << 20
	// MaxMomentsPerMonth caps one month of the timeline.
	MaxMomentsPerMonth = 500
)

var (
	// ErrMomentDay means a moment set after today.
	ErrMomentDay = fmt.Errorf("%w: a moment can't be after today", ErrInvalid)
	// ErrPhoto means a photo that isn't a JPEG of at most MaxPhotoBytes, or one on a gratitude line.
	ErrPhoto = fmt.Errorf("%w: a photo must be a JPEG of at most %d MB, on a moment", ErrInvalid, MaxPhotoBytes>>20)
)

// MomentKind is what a moment is.
type MomentKind string

const (
	// KindMoment is something that happened, with a line and maybe a photo.
	KindMoment MomentKind = "moment"
	// KindGratitude is one person's gratitude line for one day.
	KindGratitude MomentKind = "gratitude"
)

// Moment is what a household keeps on its timeline: a moment or a gratitude line. It shares the
// visibility model of entries; a "reveal" moment is a letter that opens at RevealAt.
type Moment struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	AuthorID    uuid.UUID
	Kind        MomentKind
	MomentFields
	// PhotoAt is when the photo was last set; nil means there is none.
	PhotoAt   *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	// Sealed means the caller may not read Text or see the photo yet; Text is then empty.
	Sealed bool
}

// MomentFields are what the author chooses.
type MomentFields struct {
	// Day is the day it happened, as midnight UTC of that date.
	Day        time.Time
	Visibility Visibility
	RevealAt   *time.Time
	Text       string
}

// MomentMonth is one month of the timeline.
type MomentMonth struct {
	// Month is its first day, as midnight UTC.
	Month   time.Time
	Moments []Moment
	// Earlier is the first day of the latest earlier month with a moment; zero when there is none.
	Earlier time.Time
}

// Photo is a moment's picture.
type Photo struct {
	ContentType string
	Data        []byte
}
