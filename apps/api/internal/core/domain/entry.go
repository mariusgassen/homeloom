package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MaxNoteText     = 5000
	MaxReminderText = 200
	MaxEventText    = 200
	// MaxAlertMinutes is the earliest an event's alert can go out: a week before.
	MaxAlertMinutes = 7 * 24 * 60
	// DoneReminderDays is how long a done reminder stays listed.
	DoneReminderDays = 7
)

var (
	ErrRevealAt  = fmt.Errorf("%w: a reveal entry needs revealAt, and only notes and moments can be revealed later", ErrInvalid)
	ErrAssignee  = fmt.Errorf("%w: assignee must be a member and the reminder shared", ErrInvalid)
	ErrEventTime = fmt.Errorf("%w: an event needs a start, an end not before it, and an alert of 0-%d minutes before", ErrInvalid, MaxAlertMinutes)
	// ErrEntryFields means a field that doesn't fit the kind, such as a due time on a note.
	ErrEntryFields = fmt.Errorf("%w: field does not fit this kind of entry", ErrInvalid)
)

// TextError is an entry text that is empty or longer than Max characters.
type TextError struct{ Max int }

func (e TextError) Error() string {
	return fmt.Sprintf("invalid input: text must be 1-%d characters", e.Max)
}
func (e TextError) Unwrap() error { return ErrInvalid }

// EntryKind is what an entry is.
type EntryKind string

const (
	KindNote     EntryKind = "note"
	KindReminder EntryKind = "reminder"
	KindEvent    EntryKind = "event"
)

// Visibility says who sees an entry: only its author, every member, or every member once RevealAt
// has passed (until then the others only see that it exists).
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityShared  Visibility = "shared"
	VisibilityReveal  Visibility = "reveal"
)

type Repeat string

const (
	RepeatNever   Repeat = "never"
	RepeatDaily   Repeat = "daily"
	RepeatWeekly  Repeat = "weekly"
	RepeatMonthly Repeat = "monthly"
	RepeatYearly  Repeat = "yearly"
)

// Entry is something a member writes for themselves or the household: a note, a reminder or a
// calendar event.
type Entry struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	AuthorID    uuid.UUID
	Kind        EntryKind
	EntryFields
	DoneAt *time.Time
	DoneBy *uuid.UUID
	// NotifiedAt is when the last notification (a reminder due, an event's alert) went out.
	NotifiedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// ICalUID and ICalName are the UID and resource name a calendar app gave an entry it made;
	// empty for entries made in Homeloom.
	ICalUID  string
	ICalName string
	// Sealed means the caller may not read Text yet; it is then empty.
	Sealed bool
}

// EntryFields are what the author (or, for shared entries, any member) chooses.
type EntryFields struct {
	Visibility Visibility
	RevealAt   *time.Time
	Text       string
	// DueAt is when a reminder is due. For an event it is set by the service: when its next alert
	// goes out.
	DueAt *time.Time
	// StartsAt and EndsAt are an event's time. For an all-day event they are local midnight of its
	// first and last day; EndsAt may be nil for a single moment or day.
	StartsAt     *time.Time
	EndsAt       *time.Time
	AllDay       bool
	AlertMinutes *int
	Repeat       Repeat
	// AssigneeID is who a shared reminder is for; nil means everyone.
	AssigneeID *uuid.UUID
}

// SyncList is one list a calendar app syncs: the household's shared events or reminders, or the
// actor's private ones.
type SyncList struct {
	Kind       EntryKind
	Visibility Visibility
}

// SyncEntry is an event or reminder as a calendar app sends it.
type SyncEntry struct {
	// UID is the app's id for it, kept for entries the app makes.
	UID string
	EntryFields
	// Done is whether a reminder is ticked off.
	Done bool
}

// MaxICalName is the longest resource name a calendar app may give an entry.
const MaxICalName = 200

// SyncName is the resource name calendar apps know the entry by.
func (e Entry) SyncName() string {
	if e.ICalName != "" {
		return e.ICalName
	}
	return e.ID.String() + ".ics"
}

// CalendarFeed lets a calendar app subscribe to what one member of a household can see. Only a
// hash of its token is stored.
type CalendarFeed struct {
	UserID      uuid.UUID
	HouseholdID uuid.UUID
}

// Next is the first time after now that a reminder repeating r from due falls on, in loc, so
// "every day at 8" stays at 8 across daylight saving time. It returns due for RepeatNever.
func (r Repeat) Next(due, now time.Time, loc *time.Location) time.Time {
	due = due.In(loc)
	var years, months, days int
	switch r {
	case RepeatDaily:
		days = 1
	case RepeatWeekly:
		days = 7
	case RepeatMonthly:
		months = 1
	case RepeatYearly:
		years = 1
	default:
		return due
	}
	// Stepping from the original time each round means a short month doesn't move later ones.
	next := due
	for k := 1; !next.After(now); k++ {
		next = due.AddDate(k*years, k*months, k*days)
	}
	return next
}
