package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) CreateMCPToken(ctx context.Context, tokenHash []byte, t domain.MCPToken) (domain.MCPToken, error) {
	var id *uuid.UUID
	if t.ID != uuid.Nil {
		id = &t.ID
	}
	row, err := s.q.CreateMCPToken(ctx, sqlcgen.CreateMCPTokenParams{ID: id, TokenHash: tokenHash, UserID: t.UserID, HouseholdID: t.HouseholdID, Name: t.Name, Access: string(t.Access)})
	if err != nil {
		return domain.MCPToken{}, err
	}
	return domain.MCPToken{ID: row.ID, UserID: row.UserID, HouseholdID: row.HouseholdID, Name: row.Name, Access: domain.TokenAccess(row.Access), CreatedAt: row.CreatedAt}, nil
}

func (s *Store) ListMCPTokens(ctx context.Context, householdID, userID uuid.UUID) ([]domain.MCPToken, error) {
	rows, err := s.q.ListMCPTokens(ctx, sqlcgen.ListMCPTokensParams{HouseholdID: householdID, UserID: userID})
	if err != nil {
		return nil, err
	}
	out := make([]domain.MCPToken, len(rows))
	for i, r := range rows {
		out[i] = domain.MCPToken{ID: r.ID, UserID: r.UserID, HouseholdID: r.HouseholdID, Name: r.Name, Access: domain.TokenAccess(r.Access), CreatedAt: r.CreatedAt}
	}
	return out, nil
}

func (s *Store) DeleteMCPToken(ctx context.Context, id, userID uuid.UUID) error {
	n, err := s.q.DeleteMCPToken(ctx, sqlcgen.DeleteMCPTokenParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) MCPTokenByHash(ctx context.Context, tokenHash []byte) (domain.MCPToken, domain.User, error) {
	r, err := s.q.GetMCPTokenByHash(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MCPToken{}, domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.MCPToken{}, domain.User{}, err
	}
	t := domain.MCPToken{ID: r.ID, UserID: r.UserID, HouseholdID: r.HouseholdID, Name: r.Name, Access: domain.TokenAccess(r.Access), CreatedAt: r.CreatedAt}
	return t, domain.User{ID: r.UserID, Name: r.UserName, Email: deref(r.Email), AvatarURL: deref(r.AvatarUrl)}, nil
}

func (s *Store) MCPTokenByID(ctx context.Context, id uuid.UUID) (domain.MCPToken, domain.User, []byte, error) {
	r, err := s.q.GetMCPTokenByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MCPToken{}, domain.User{}, nil, domain.ErrNotFound
	}
	if err != nil {
		return domain.MCPToken{}, domain.User{}, nil, err
	}
	t := domain.MCPToken{ID: r.ID, UserID: r.UserID, HouseholdID: r.HouseholdID, Name: r.Name, Access: domain.TokenAccess(r.Access), CreatedAt: r.CreatedAt}
	return t, domain.User{ID: r.UserID, Name: r.UserName, Email: deref(r.Email), AvatarURL: deref(r.AvatarUrl)}, r.TokenHash, nil
}
