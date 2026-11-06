package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MaxGiftText = 200
	// MaxGiftIdeas is how many ideas one member keeps.
	MaxGiftIdeas = 300
	// GiftHorizonDays is how many days ahead a birthday is surfaced with its gift ideas.
	GiftHorizonDays = 21
)

// ErrBirthday means a month and day that don't exist.
var ErrBirthday = fmt.Errorf("%w: not a date", ErrInvalid)

// MinBirthYear is the earliest year of birth accepted.
const MinBirthYear = 1900

// Birthday is a day of the year, with the year of birth when the member gave it.
type Birthday struct {
	Month time.Month
	Day   int
	// Year is the year of birth; 0 when not given.
	Year int
}

// Valid reports whether the day exists: in some year without a Year (29 February does), in that
// year with one, which must be from MinBirthYear up to this year.
func (b Birthday) Valid() bool {
	if b.Month < time.January || b.Month > time.December || b.Day < 1 {
		return false
	}
	year := 2000 // a leap year
	if b.Year != 0 {
		if b.Year < MinBirthYear || b.Year > time.Now().Year() {
			return false
		}
		year = b.Year
	}
	return b.Day <= time.Date(year, b.Month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// Age is how old the member turns on the birthday that falls on next; 0 without a year.
func (b Birthday) Age(next time.Time) int {
	if b.Year == 0 {
		return 0
	}
	return next.Year() - b.Year
}

// Next returns the first day on or after today on which the birthday falls, and how many days
// away that is. Only the date of today counts. In years without a 29 February it falls on the 28th.
func (b Birthday) Next(today time.Time) (time.Time, int) {
	y := today.Year()
	for {
		d := time.Date(y, b.Month, b.Day, 0, 0, 0, 0, time.UTC)
		if d.Month() != b.Month { // 29 February in a common year
			d = time.Date(y, b.Month, b.Day-1, 0, 0, 0, 0, time.UTC)
		}
		t := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
		if !d.Before(t) {
			return d, int(d.Sub(t).Hours() / 24)
		}
		y++
	}
}

// GiftIdea is something one member thought of giving another. Only its author ever sees it.
type GiftIdea struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	AuthorID    uuid.UUID
	ForID       uuid.UUID
	Text        string
	// GivenAt is when the author marked it as given; nil while it's still an idea.
	GivenAt *time.Time
	Created time.Time
}

// GiftPerson is a member the caller can give something to, with their birthday if they set one.
type GiftPerson struct {
	User     User
	Birthday *Birthday
	// Next is the next birthday and Days how many days away it is; both are zero without a birthday.
	Next time.Time
	Days int
	// Ideas is how many of the caller's own open ideas are for this person.
	Ideas int
}

// Gifts is what the caller sees: their own birthday, the others, and only their own ideas.
type Gifts struct {
	MyBirthday *Birthday
	People     []GiftPerson
	Ideas      []GiftIdea
}

// ErrGiftFor means an idea for oneself or for someone outside the household.
var ErrGiftFor = fmt.Errorf("%w: an idea is for another member", ErrInvalid)
