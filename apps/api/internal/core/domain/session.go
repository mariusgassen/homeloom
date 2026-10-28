package domain

import (
	"time"

	"github.com/google/uuid"
)

// PendingLogin is a sign-in that was started and waits for authentik to redirect back.
type PendingLogin struct {
	Nonce    string
	Verifier string // PKCE code verifier
}

// Session is a signed-in browser. It keeps the identity from the ID token, so requests
// made with a session need no call to authentik.
type Session struct {
	Identity Identity
	IDToken  string // sent back to authentik as a hint when signing out
}

const (
	// AgentTokenPrefix marks Homeloom's own agent tokens, so they are never sent to authentik.
	AgentTokenPrefix = "hla_"
	// MaxAgentTokenTTL bounds how long one agent token lasts, such as for one long call.
	MaxAgentTokenTTL = 6 * time.Hour
)

// AgentGrant lets a helper such as the voice agent use the tools as one member of one household
// until it expires. The token that names it is only given to that helper.
type AgentGrant struct {
	User        User
	HouseholdID uuid.UUID
	ExpiresAt   time.Time
}
