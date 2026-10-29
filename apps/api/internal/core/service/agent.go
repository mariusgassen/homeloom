package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// AgentAccess hands out the tokens with which another app or a helper uses the tools as one
// member of one household: short-lived ones minted for a helper such as the voice agent, and
// personal ones a member makes for an app. Only a hash of each token is stored: SHA-256 for the
// keys, which are looked up by it, and bcrypt for passwords.
type AgentAccess struct {
	households port.HouseholdRepository
	sessions   port.SessionStore
	tokens     port.MCPTokenRepository
	// checked remembers the password last checked for each token id.
	checked sync.Map
	now     func() time.Time
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
		return s.personal(ctx, token)
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

// AuthenticatePassword takes the password a calendar or notes app sends: the access's prefix, the
// token's id and a secret kept only as a bcrypt hash. A password that has checked out is
// remembered in memory, so a syncing app doesn't pay for bcrypt on every request; the token is
// still looked up each time, so a removed one stops working at once.
func (s *AgentAccess) AuthenticatePassword(ctx context.Context, access domain.TokenAccess, password string) (domain.AgentGrant, error) {
	id, secret, ok := splitPassword(access, password)
	if !ok {
		return domain.AgentGrant{}, fmt.Errorf("%w: not a %s password", domain.ErrUnauthenticated, access)
	}
	t, owner, hash, err := s.tokens.MCPTokenByID(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.AgentGrant{}, fmt.Errorf("%w: unknown token", domain.ErrUnauthenticated)
	}
	if err != nil {
		return domain.AgentGrant{}, err
	}
	if t.Access != access {
		return domain.AgentGrant{}, fmt.Errorf("%w: token made for %s", domain.ErrUnauthenticated, t.Access)
	}
	if known, _ := s.checked.Load(id); known == nil || subtle.ConstantTimeCompare([]byte(known.(string)), []byte(password)) != 1 {
		if bcrypt.CompareHashAndPassword(hash, []byte(secret)) != nil {
			return domain.AgentGrant{}, fmt.Errorf("%w: wrong password", domain.ErrUnauthenticated)
		}
		s.checked.Store(id, password)
	}
	return domain.AgentGrant{User: owner, HouseholdID: t.HouseholdID}, nil
}

// passwordIDLen is the length of a token id in a password.
var passwordIDLen = base64.RawURLEncoding.EncodedLen(len(uuid.UUID{}))

func splitPassword(access domain.TokenAccess, password string) (uuid.UUID, string, bool) {
	prefix := access.PasswordPrefix()
	rest, ok := strings.CutPrefix(password, prefix)
	if prefix == "" || !ok || len(rest) <= passwordIDLen {
		return uuid.Nil, "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(rest[:passwordIDLen])
	if err != nil {
		return uuid.Nil, "", false
	}
	id, err := uuid.FromBytes(b)
	return id, rest[passwordIDLen:], err == nil
}

// personal looks up a personal key for the tools.
func (s *AgentAccess) personal(ctx context.Context, token string) (domain.AgentGrant, error) {
	t, owner, err := s.tokens.MCPTokenByHash(ctx, hashCode(token))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.AgentGrant{}, fmt.Errorf("%w: unknown token", domain.ErrUnauthenticated)
	}
	if err != nil {
		return domain.AgentGrant{}, err
	}
	if t.Access != domain.AccessTools {
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
	t := domain.MCPToken{UserID: actor.ID, HouseholdID: householdID, Name: name, Access: access}
	var secret string
	var hash []byte
	var err error
	switch access {
	case domain.AccessTools:
		if secret, err = newSecret(domain.MCPTokenPrefix); err != nil {
			return domain.MCPToken{}, "", err
		}
		hash = hashCode(secret)
	case domain.AccessCalendar, domain.AccessNotes:
		// A password names its token, so it is checked against that token's bcrypt hash.
		t.ID = uuid.New()
		random, err := newSecret("")
		if err != nil {
			return domain.MCPToken{}, "", err
		}
		if hash, err = bcrypt.GenerateFromPassword([]byte(random), bcrypt.DefaultCost); err != nil {
			return domain.MCPToken{}, "", err
		}
		secret = access.PasswordPrefix() + base64.RawURLEncoding.EncodeToString(t.ID[:]) + random
	default:
		return domain.MCPToken{}, "", fmt.Errorf("%w: unknown access %q", domain.ErrInvalid, access)
	}
	if t, err = s.tokens.CreateMCPToken(ctx, hash, t); err != nil {
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
