package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	// MCPTokenPrefix marks personal tokens for other apps, so they are never sent to authentik.
	MCPTokenPrefix = "hlp_"
	// CalendarPasswordPrefix marks personal passwords for calendar apps.
	CalendarPasswordPrefix = "hlc_"
	MaxMCPTokenName        = 60
)

// TokenAccess is what a personal token opens. A token opens only that.
type TokenAccess string

const (
	// AccessTools is the assistant's tools at /mcp, as a bearer token.
	AccessTools TokenAccess = "tools"
	// AccessCalendar is events and reminders over CalDAV at /dav, as a password.
	AccessCalendar TokenAccess = "calendar"
)

// MCPToken lets another app act as its owner in one household, until it is deleted: an assistant
// through the tools, or a calendar app through CalDAV.
type MCPToken struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	HouseholdID uuid.UUID
	Name        string
	Access      TokenAccess
	CreatedAt   time.Time
}
