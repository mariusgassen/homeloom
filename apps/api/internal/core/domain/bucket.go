package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MaxBucketText = 200
	// MaxBucketItems is how many bucket the list shows.
	MaxBucketItems = 500
)

// BucketKind is what a item is for: something to watch, to read or a place to visit.
type BucketKind string

const (
	BucketWatch BucketKind = "watch"
	BucketRead  BucketKind = "read"
	BucketVisit BucketKind = "visit"
)

func (k BucketKind) Valid() bool {
	return k == BucketWatch || k == BucketRead || k == BucketVisit
}

// ErrBucketKind is a item of a kind there is none of.
var ErrBucketKind = fmt.Errorf("%w: unknown kind", ErrInvalid)

// BucketItem is one thing the household wants to watch, read or visit some day. Once ticked off it is
// done: watched, read or been there.
type BucketItem struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	Kind        BucketKind
	Text        string
	AddedBy     *uuid.UUID
	DoneAt      *time.Time
	Created     time.Time
}
