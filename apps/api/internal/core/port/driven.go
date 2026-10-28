// Package port defines the boundaries of the core: driving ports are implemented by the core
// and called by adapters (HTTP, MCP, jobs); driven ports are implemented by adapters
// (Postgres, Redis, authentik) and called by the core.
package port

import (
	"context"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

type TokenVerifier interface {
	// Verify returns domain.ErrUnauthenticated for any token that is not acceptable.
	Verify(ctx context.Context, rawToken string) (domain.Identity, error)
}

// LoginProvider runs the authorization code flow for the confidential web client.
type LoginProvider interface {
	AuthCodeURL(ctx context.Context, state, nonce, verifier string) (string, error)
	// Exchange redeems the code and returns the identity and raw ID token. It returns
	// domain.ErrUnauthenticated for a code or ID token that is not acceptable.
	Exchange(ctx context.Context, code, verifier, nonce string) (domain.Identity, string, error)
	// LogoutURL is where to send the browser to sign out of the provider too; "" if it has none.
	LogoutURL(ctx context.Context, idToken string) (string, error)
}

// SessionStore keeps pending logins and sessions; it decides how long each lives.
type SessionStore interface {
	SaveLogin(ctx context.Context, state string, l domain.PendingLogin) error
	// TakeLogin returns and removes a pending login, so a state works once. It returns
	// domain.ErrNotFound for an unknown or expired state.
	TakeLogin(ctx context.Context, state string) (domain.PendingLogin, error)
	Save(ctx context.Context, id string, s domain.Session) error
	// Get returns domain.ErrNotFound for an unknown or expired session.
	Get(ctx context.Context, id string) (domain.Session, error)
	Delete(ctx context.Context, id string) error
}

type UserRepository interface {
	UpsertByIdentity(ctx context.Context, id domain.Identity) (domain.User, error)
}

type HouseholdRepository interface {
	// Create stores the household and makes owner its first member, atomically.
	Create(ctx context.Context, name string, owner uuid.UUID) (domain.Membership, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]domain.Membership, error)
	// Role returns domain.ErrNotFound when the user is not a member.
	Role(ctx context.Context, householdID, userID uuid.UUID) (domain.Role, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Household, error)
	Members(ctx context.Context, householdID uuid.UUID) ([]domain.Member, error)
}

type HealthCheck interface {
	Name() string
	Check(ctx context.Context) error
}
