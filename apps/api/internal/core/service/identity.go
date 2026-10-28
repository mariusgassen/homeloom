// Package service implements the driving ports. All authorisation decisions live here,
// so every adapter (HTTP, MCP, jobs) gets the same rules.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

type Identity struct {
	verifier port.TokenVerifier
	sessions port.SessionStore
	users    port.UserRepository
}

func NewIdentity(verifier port.TokenVerifier, sessions port.SessionStore, users port.UserRepository) *Identity {
	return &Identity{verifier: verifier, sessions: sessions, users: users}
}

func (s *Identity) Authenticate(ctx context.Context, rawToken string) (domain.User, error) {
	if rawToken == "" {
		return domain.User{}, domain.ErrUnauthenticated
	}
	id, err := s.verifier.Verify(ctx, rawToken)
	if err != nil {
		return domain.User{}, err
	}
	return s.user(ctx, id)
}

func (s *Identity) AuthenticateSession(ctx context.Context, sessionID string) (domain.User, error) {
	if sessionID == "" {
		return domain.User{}, domain.ErrUnauthenticated
	}
	sess, err := s.sessions.Get(ctx, sessionID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, fmt.Errorf("%w: unknown session", domain.ErrUnauthenticated)
	}
	if err != nil {
		return domain.User{}, err
	}
	return s.user(ctx, sess.Identity)
}

func (s *Identity) user(ctx context.Context, id domain.Identity) (domain.User, error) {
	if id.Subject == "" {
		return domain.User{}, fmt.Errorf("%w: token has no subject", domain.ErrUnauthenticated)
	}
	if id.Name == "" {
		id.Name = id.Email
	}
	return s.users.UpsertByIdentity(ctx, id)
}
