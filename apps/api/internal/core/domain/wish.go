package domain

import (
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
)

const (
	MaxWishTitle = 100
	MaxWishNote  = 500
	MaxWishURL   = 2000
	// MaxWishPriceCents is the largest price a wish may have (1,000,000.00).
	MaxWishPriceCents = 100_000_000
	// MaxWishHearts is how much a member can want a wish; the least is 1.
	MaxWishHearts = 3
	// MaxWishes is how many wishes a household lists.
	MaxWishes = 500
)

var (
	ErrWishNote   = fmt.Errorf("%w: a note may have at most %d characters", ErrInvalid, MaxWishNote)
	ErrWishLink   = fmt.Errorf("%w: a link must be a web address of at most %d characters", ErrInvalid, MaxWishURL)
	ErrWishPrice  = fmt.Errorf("%w: a price must be 0.01-%d.00", ErrInvalid, MaxWishPriceCents/100)
	ErrWishHearts = fmt.Errorf("%w: hearts must be 1-%d", ErrInvalid, MaxWishHearts)
	ErrWishPhoto  = fmt.Errorf("%w: a photo must be a JPEG of at most %d MB", ErrInvalid, MaxPhotoBytes>>20)
)

// Wish is something the household would like for the home. Members rate it in hearts; the
// wishes that are wanted most come first.
type Wish struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	WishFields
	// PhotoAt is when the photo was kept in a PhotoStore; nil when there is none.
	PhotoAt *time.Time
	// GotAt is when the wish came true; nil while it is still a wish.
	GotAt     *time.Time
	Votes     []WishVote
	CreatedBy *uuid.UUID
	CreatedAt time.Time
}

// WishFields are what any member chooses.
type WishFields struct {
	Title string
	Note  string
	// URL is where to see or buy it; empty or an http(s) address.
	URL        string
	PriceCents *int64
}

// WishVote is how much one member wants a wish.
type WishVote struct {
	UserID uuid.UUID
	Hearts int
}

// Hearts is the sum of all the members' hearts.
func (w Wish) Hearts() int {
	n := 0
	for _, v := range w.Votes {
		n += v.Hearts
	}
	return n
}

// WebLink reports whether s is an absolute http or https address.
func WebLink(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
