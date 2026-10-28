// Package service implements the driving ports. All authorisation decisions live here,
// so every adapter (HTTP, MCP, jobs) gets the same rules.
package service

import (
	"context"
	"fmt"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

type Identity struct {
	verifier port.TokenVerifier
	users    port.UserRepository
}

func NewIdentity(verifier port.TokenVerifier, users port.UserRepository) *Identity {
	return &Identity{verifier: verifier, users: users}
}

func (s *Identity) Authenticate(ctx context.Context, rawToken string) (domain.User, error) {
	if rawToken == "" {
		return domain.User{}, domain.ErrUnauthenticated
	}
	id, err := s.verifier.Verify(ctx, rawToken)
	if err != nil {
		return domain.User{}, err
	}
	if id.Subject == "" {
		return domain.User{}, fmt.Errorf("%w: token has no subject", domain.ErrUnauthenticated)
	}
	if id.Name == "" {
		id.Name = id.Email
	}
	return s.users.UpsertByIdentity(ctx, id)
}
