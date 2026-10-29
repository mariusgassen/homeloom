package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	// MCPTokenPrefix marks personal tokens for other apps, so they are never sent to authentik.
	MCPTokenPrefix = "hlp_"
	// CalendarPasswordPrefix and NotesPasswordPrefix mark personal passwords for calendar and
	// notes apps.
	CalendarPasswordPrefix = "hlc_"
	NotesPasswordPrefix    = "hln_"
	MaxMCPTokenName        = 60
)

// TokenAccess is what a personal token opens. A token opens only that.
type TokenAccess string

const (
	// AccessTools is the assistant's tools at /mcp, as a bearer token.
	AccessTools TokenAccess = "tools"
	// AccessCalendar is events and reminders over CalDAV at /dav, as a password.
	AccessCalendar TokenAccess = "calendar"
	// AccessNotes is notes as files over WebDAV at /webdav, as a password.
	AccessNotes TokenAccess = "notes"
)

// PasswordPrefix is what a password for the access starts with; "" means the access takes a key.
func (a TokenAccess) PasswordPrefix() string {
	switch a {
	case AccessCalendar:
		return CalendarPasswordPrefix
	case AccessNotes:
		return NotesPasswordPrefix
	}
	return ""
}

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
