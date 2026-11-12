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
	// MaxOccasionName and MaxOccasions bound the dates a household keeps besides birthdays.
	MaxOccasionName = 60
	MaxOccasions    = 100
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
	// OccasionID is the occasion the idea is for; nil when it isn't tied to one.
	OccasionID *uuid.UUID
	Text       string
	// GivenAt is when the author marked it as given; nil while it's still an idea.
	GivenAt *time.Time
	Created time.Time
}

// Occasion is a date the household keeps, such as Christmas, a wedding anniversary or a weekly
// game night. On is the day it first falls (or falls once); Repeat says how it comes back.
type Occasion struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	Name        string
	On          time.Time
	Repeat      Repeat
	Created     time.Time
}

// OccasionRepeat reports whether an occasion may repeat this way: not at all, weekly, monthly or
// yearly.
func OccasionRepeat(r Repeat) bool {
	return r == RepeatNever || r == RepeatWeekly || r == RepeatMonthly || r == RepeatYearly
}

// Next returns the first day on or after today on which the occasion falls, and how many days
// away that is. ok is false for a one-off whose day has passed. A monthly occasion on the 31st
// falls on the last day of shorter months; a yearly one on 29 February on the 28th in common years.
func (o Occasion) Next(today time.Time) (next time.Time, days int, ok bool) {
	t := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	on := time.Date(o.On.Year(), o.On.Month(), o.On.Day(), 0, 0, 0, 0, time.UTC)
	from := t
	if on.After(t) {
		from = on // it hasn't started yet
	}
	switch o.Repeat {
	case RepeatWeekly:
		next = from.AddDate(0, 0, (int(on.Weekday())-int(from.Weekday())+7)%7)
	case RepeatMonthly:
		next = monthlyOn(from.Year(), from.Month(), on.Day())
		if next.Before(from) {
			next = monthlyOn(from.Year(), from.Month()+1, on.Day())
		}
	case RepeatYearly:
		next, _ = Birthday{Month: on.Month(), Day: on.Day()}.Next(from)
	default:
		if on.Before(t) {
			return time.Time{}, 0, false
		}
		next = on
	}
	return next, int(next.Sub(t).Hours() / 24), true
}

// monthlyOn is the day of the month, or the last day of a month that is shorter.
func monthlyOn(year int, month time.Month, day int) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	if day > last.Day() {
		return last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// GiftOccasion is an occasion with when it next falls.
type GiftOccasion struct {
	Occasion
	Next time.Time
	Days int
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
	// Occasions are the household's other yearly dates, soonest first.
	Occasions []GiftOccasion
	People    []GiftPerson
	Ideas     []GiftIdea
}

// ErrOccasionRepeat means a repeat an occasion can't have.
var ErrOccasionRepeat = fmt.Errorf("%w: an occasion repeats weekly, monthly, yearly or not at all", ErrInvalid)

// ErrOccasion means an idea for an occasion the household doesn't have.
var ErrOccasion = fmt.Errorf("%w: not an occasion of this household", ErrInvalid)

// ErrGiftFor means an idea for oneself or for someone outside the household.
var ErrGiftFor = fmt.Errorf("%w: an idea is for another member", ErrInvalid)
