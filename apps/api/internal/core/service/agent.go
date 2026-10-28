package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// AgentAccess hands out the tokens with which another app or a helper uses the tools as one
// member of one household: short-lived ones minted for a helper such as the voice agent, and
// personal ones a member makes for an app. Only a hash of each token is stored.
type AgentAccess struct {
	households port.HouseholdRepository
	sessions   port.SessionStore
	tokens     port.MCPTokenRepository
	now        func() time.Time
}

func NewAgentAccess(households port.HouseholdRepository, sessions port.SessionStore, tokens port.MCPTokenRepository) *AgentAccess {
	return &AgentAccess{households: households, sessions: sessions, tokens: tokens, now: time.Now}
}

func (s *AgentAccess) Mint(ctx context.Context, actor domain.User, householdID uuid.UUID, ttl time.Duration) (string, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return "", err
	}
	if ttl <= 0 || ttl > domain.MaxAgentTokenTTL {
		return "", fmt.Errorf("%w: a token lasts up to %s", domain.ErrInvalid, domain.MaxAgentTokenTTL)
	}
	token, err := newSecret(domain.AgentTokenPrefix)
	if err != nil {
		return "", err
	}
	g := domain.AgentGrant{User: actor, HouseholdID: householdID, ExpiresAt: s.now().Add(ttl)}
	if err := s.sessions.SaveAgentGrant(ctx, token, g); err != nil {
		return "", err
	}
	return token, nil
}

func (s *AgentAccess) Authenticate(ctx context.Context, token string) (domain.AgentGrant, error) {
	if strings.HasPrefix(token, domain.MCPTokenPrefix) {
		return s.personal(ctx, token, domain.AccessTools)
	}
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
	if g.ExpiresAt.IsZero() || !s.now().Before(g.ExpiresAt) {
		return domain.AgentGrant{}, fmt.Errorf("%w: agent token expired", domain.ErrUnauthenticated)
	}
	return g, nil
}

func (s *AgentAccess) AuthenticateCalendar(ctx context.Context, password string) (domain.AgentGrant, error) {
	if !strings.HasPrefix(password, domain.CalendarPasswordPrefix) {
		return domain.AgentGrant{}, fmt.Errorf("%w: not a calendar password", domain.ErrUnauthenticated)
	}
	return s.personal(ctx, password, domain.AccessCalendar)
}

// personal looks up a personal token, which only opens what it was made for.
func (s *AgentAccess) personal(ctx context.Context, token string, access domain.TokenAccess) (domain.AgentGrant, error) {
	t, owner, err := s.tokens.MCPTokenByHash(ctx, hashCode(token))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.AgentGrant{}, fmt.Errorf("%w: unknown token", domain.ErrUnauthenticated)
	}
	if err != nil {
		return domain.AgentGrant{}, err
	}
	if t.Access != access {
		return domain.AgentGrant{}, fmt.Errorf("%w: token made for %s", domain.ErrUnauthenticated, t.Access)
	}
	return domain.AgentGrant{User: owner, HouseholdID: t.HouseholdID}, nil
}

func (s *AgentAccess) Revoke(ctx context.Context, token string) error {
	return s.sessions.DeleteAgentGrant(ctx, token)
}

func (s *AgentAccess) Tokens(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.MCPToken, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.tokens.ListMCPTokens(ctx, householdID, actor.ID)
}

func (s *AgentAccess) CreateToken(ctx context.Context, actor domain.User, householdID uuid.UUID, name string, access domain.TokenAccess) (domain.MCPToken, string, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.MCPToken{}, "", err
	}
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || utf8.RuneCountInString(name) > domain.MaxMCPTokenName {
		return domain.MCPToken{}, "", domain.TextError{Max: domain.MaxMCPTokenName}
	}
	prefix := domain.MCPTokenPrefix
	switch access {
	case domain.AccessTools:
	case domain.AccessCalendar:
		prefix = domain.CalendarPasswordPrefix
	default:
		return domain.MCPToken{}, "", fmt.Errorf("%w: unknown access %q", domain.ErrInvalid, access)
	}
	secret, err := newSecret(prefix)
	if err != nil {
		return domain.MCPToken{}, "", err
	}
	t, err := s.tokens.CreateMCPToken(ctx, hashCode(secret), domain.MCPToken{UserID: actor.ID, HouseholdID: householdID, Name: name, Access: access})
	if err != nil {
		return domain.MCPToken{}, "", err
	}
	return t, secret, nil
}

// DeleteToken deletes one of the actor's tokens; anyone else's is domain.ErrNotFound.
func (s *AgentAccess) DeleteToken(ctx context.Context, actor domain.User, id uuid.UUID) error {
	return s.tokens.DeleteMCPToken(ctx, id, actor.ID)
}

func newSecret(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}
