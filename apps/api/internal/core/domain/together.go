package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	MaxQuestionText = 280
	MaxAnswerText   = 2000
	// InviteTTL is how long an invite link works.
	InviteTTL = 7 * 24 * time.Hour
)

// Invite lets someone join a household. Only a hash of Code is stored.
type Invite struct {
	Code      string
	ExpiresAt time.Time
}

// Question is asked to the whole household. Answers stay sealed from the other members until
// every member has answered; then the question is revealed and its answers are fixed.
type Question struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	AskedBy     uuid.UUID
	Text        string
	CreatedAt   time.Time
	RevealedAt  *time.Time
	Answers     []Answer
}

func (q Question) Revealed() bool { return q.RevealedAt != nil }

type Answer struct {
	UserID     uuid.UUID
	Text       string
	AnsweredAt time.Time
	// Sealed means the caller may not read Text yet; it is then empty.
	Sealed bool
}
