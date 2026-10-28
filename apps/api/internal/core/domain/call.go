package domain

import (
	"time"

	"github.com/google/uuid"
)

// MaxGuestName is the longest name a guest can join a call with.
const MaxGuestName = 50

// GuestPassTTL is how long a guest link to a call works.
const GuestPassTTL = 12 * time.Hour

// Call is what a client needs to join a call: which call, the LiveKit server it connects to and
// an access token for the call's room.
type Call struct {
	ID    uuid.UUID
	URL   string
	Token string
	Room  string
}

// GuestPass lets someone without an account into one call; it is stored under a hash of its link.
type GuestPass struct {
	HouseholdID uuid.UUID
	CallID      uuid.UUID
	// InvitedBy is the name of the member who made the link, to show the guest.
	InvitedBy string
}

// GuestLink is a new guest pass: Token goes into the link the member shares.
type GuestLink struct {
	Token     string
	ExpiresAt time.Time
}
