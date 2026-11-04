package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MaxWishText = 200
	// MaxWishes is how many wishes the list shows.
	MaxWishes = 500
)

// WishKind is what a wish is for: something to watch, to read or a place to visit.
type WishKind string

const (
	WishWatch WishKind = "watch"
	WishRead  WishKind = "read"
	WishVisit WishKind = "visit"
)

func (k WishKind) Valid() bool {
	return k == WishWatch || k == WishRead || k == WishVisit
}

// ErrWishKind is a wish of a kind there is none of.
var ErrWishKind = fmt.Errorf("%w: unknown kind", ErrInvalid)

// Wish is one thing the household wants to watch, read or visit some day. Once ticked off it is
// done: watched, read or been there.
type Wish struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	Kind        WishKind
	Text        string
	AddedBy     *uuid.UUID
	DoneAt      *time.Time
	Created     time.Time
}
