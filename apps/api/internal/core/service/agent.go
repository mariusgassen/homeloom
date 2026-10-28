package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// AgentAccess hands out short-lived tokens with which a helper uses the tools as one member of one
// household. Only a hash of each token is stored.
type AgentAccess struct {
	households port.HouseholdRepository
	sessions   port.SessionStore
	now        func() time.Time
}

func NewAgentAccess(households port.HouseholdRepository, sessions port.SessionStore) *AgentAccess {
	return &AgentAccess{households: households, sessions: sessions, now: time.Now}
}

func (s *AgentAccess) Mint(ctx context.Context, actor domain.User, householdID uuid.UUID, ttl time.Duration) (string, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return "", err
	}
	if ttl <= 0 || ttl > domain.MaxAgentTokenTTL {
		return "", fmt.Errorf("%w: a token lasts up to %s", domain.ErrInvalid, domain.MaxAgentTokenTTL)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := domain.AgentTokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	g := domain.AgentGrant{User: actor, HouseholdID: householdID, ExpiresAt: s.now().Add(ttl)}
	if err := s.sessions.SaveAgentGrant(ctx, token, g); err != nil {
		return "", err
	}
	return token, nil
}

func (s *AgentAccess) Authenticate(ctx context.Context, token string) (domain.AgentGrant, error) {
	if !strings.HasPrefix(token, domain.AgentTokenPrefix) {
		return domain.AgentGrant{}, fmt.Errorf("%w: not an agent token", domain.ErrUnauthenticated)
	}
	g, err := s.sessions.AgentGrant(ctx, token)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.AgentGrant{}, fmt.Errorf("%w: unknown agent token", domain.ErrUnauthenticated)
	}
	if err != nil {
		return domain.AgentGrant{}, err
	}
	if !s.now().Before(g.ExpiresAt) {
		return domain.AgentGrant{}, fmt.Errorf("%w: agent token expired", domain.ErrUnauthenticated)
	}
	return g, nil
}

func (s *AgentAccess) Revoke(ctx context.Context, token string) error {
	return s.sessions.DeleteAgentGrant(ctx, token)
}
