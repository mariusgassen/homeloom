package port

import (
	"context"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

type IdentityService interface {
	// Authenticate turns an access token into a local user, creating it on first sight.
	Authenticate(ctx context.Context, rawToken string) (domain.User, error)
	// AuthenticateSession does the same for a session id from the login flow.
	AuthenticateSession(ctx context.Context, sessionID string) (domain.User, error)
}

type LoginService interface {
	// Begin starts a sign-in: state is to be bound to the browser, url is where to send it.
	Begin(ctx context.Context) (state, url string, err error)
	// Complete finishes it. browserState is the state the browser was given by Begin; the
	// result is the new session's id.
	Complete(ctx context.Context, state, browserState, code string) (string, error)
	// Logout ends the session, if any, and returns the provider's sign-out URL or "".
	Logout(ctx context.Context, sessionID string) (string, error)
}

type HouseholdService interface {
	List(ctx context.Context, actor domain.User) ([]domain.Membership, error)
	Create(ctx context.Context, actor domain.User, name string) (domain.Membership, error)
	Get(ctx context.Context, actor domain.User, id uuid.UUID) (domain.HouseholdDetail, error)
}
