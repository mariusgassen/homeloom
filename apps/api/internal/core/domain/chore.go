package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MaxChoreText = 100
	// MaxChorePoints is the effort of the biggest chore; the smallest is worth 1.
	MaxChorePoints = 3
	// MaxChores is how many chores a household lists.
	MaxChores = 200
)

var (
	ErrChorePoints   = fmt.Errorf("%w: points must be 1-%d", ErrInvalid, MaxChorePoints)
	ErrChoreAssignee = fmt.Errorf("%w: assignee must be a member", ErrInvalid)
	// ErrChoreDone means a chore ticked off a second time on one day, or a one-off done already.
	ErrChoreDone = fmt.Errorf("%w: already done", ErrConflict)
	// ErrOwnChore means thanking yourself.
	ErrOwnChore = fmt.Errorf("%w: you can't thank yourself", ErrConflict)
)

// Chore is something the household does again and again, or once. A rotating chore goes to the
// next member each time it is done.
type Chore struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	ChoreFields
	// DoneAt is when a chore that doesn't repeat was done; it is then no longer listed.
	DoneAt    *time.Time
	CreatedBy *uuid.UUID
	CreatedAt time.Time
}

// ChoreFields are what any member chooses.
type ChoreFields struct {
	Title string
	// Points is the effort, 1 to MaxChorePoints.
	Points int
	Repeat Repeat
	Rotate bool
	// AssigneeID is whose turn it is; nil means anyone's.
	AssigneeID *uuid.UUID
	// DueOn is the day it is due, as midnight UTC of that date in the household's time zone.
	DueOn time.Time
}

// ChoreCompletion is one time a member did a chore. It keeps the chore's title and points, so
// history and fairness outlive the chore, and how the chore was before, so it can be undone.
type ChoreCompletion struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	// ChoreID is nil once the chore was deleted.
	ChoreID *uuid.UUID
	Title   string
	Points  int
	UserID  uuid.UUID
	// Day is the household's date it was done on, as midnight UTC.
	Day    time.Time
	DoneAt time.Time
	// PrevDueOn and PrevAssigneeID are the chore's due day and assignee before it was done.
	PrevDueOn      time.Time
	PrevAssigneeID *uuid.UUID
	// ThankedBy are the members who said thanks for it.
	ThankedBy []uuid.UUID
}

// ChoreTally is how much each member did since From.
type ChoreTally struct {
	From    time.Time
	Members []ChoreShare
}

// ChoreShare is one member's part of a tally.
type ChoreShare struct {
	UserID uuid.UUID
	Points int
	Count  int
}

// ChoreFairness is who did how much this week (from Monday) and this month.
type ChoreFairness struct {
	Week  ChoreTally
	Month ChoreTally
}
