package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	// MCPTokenPrefix marks personal tokens for other apps, so they are never sent to authentik.
	MCPTokenPrefix  = "hlp_"
	MaxMCPTokenName = 60
)

// MCPToken lets another app use the tools as its owner in one household, until it is deleted.
type MCPToken struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	HouseholdID uuid.UUID
	Name        string
	CreatedAt   time.Time
}
