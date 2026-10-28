package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

type mcpToken struct {
	hash string
	domain.MCPToken
}

func (s *Store) CreateMCPToken(_ context.Context, tokenHash []byte, t domain.MCPToken) (domain.MCPToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.ID, t.CreatedAt = uuid.New(), time.Now()
	s.mcpTokens = append(s.mcpTokens, mcpToken{hash: string(tokenHash), MCPToken: t})
	return t, nil
}

func (s *Store) ListMCPTokens(_ context.Context, householdID, userID uuid.UUID) ([]domain.MCPToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.MCPToken{}
	for _, t := range slices.Backward(s.mcpTokens) {
		if t.HouseholdID == householdID && t.UserID == userID {
			out = append(out, t.MCPToken)
		}
	}
	return out, nil
}

func (s *Store) DeleteMCPToken(_ context.Context, id, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.mcpTokens)
	s.mcpTokens = slices.DeleteFunc(s.mcpTokens, func(t mcpToken) bool { return t.ID == id && t.UserID == userID })
	if len(s.mcpTokens) == n {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) MCPTokenByHash(_ context.Context, tokenHash []byte) (domain.MCPToken, domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.mcpTokens {
		if t.hash == string(tokenHash) {
			return t.MCPToken, s.users[t.UserID], nil
		}
	}
	return domain.MCPToken{}, domain.User{}, domain.ErrNotFound
}
