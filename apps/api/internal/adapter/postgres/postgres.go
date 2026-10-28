// Package postgres implements the repository ports on PostgreSQL via sqlc-generated queries.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, mustSub(migrations, "migrations"))
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

type Store struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlcgen.New(pool)}
}

func (s *Store) UpsertByIdentity(ctx context.Context, id domain.Identity) (domain.User, error) {
	u, err := s.q.UpsertUser(ctx, sqlcgen.UpsertUserParams{
		AuthentikSub: id.Subject,
		Name:         id.Name,
		Email:        nullable(id.Email),
		AvatarUrl:    nullable(id.AvatarURL),
	})
	if err != nil {
		return domain.User{}, err
	}
	return domain.User{ID: u.ID, Name: u.Name, Email: deref(u.Email), AvatarURL: deref(u.AvatarUrl)}, nil
}

func (s *Store) Create(ctx context.Context, name string, owner uuid.UUID) (domain.Membership, error) {
	var m domain.Membership
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		h, err := q.CreateHousehold(ctx, name)
		if err != nil {
			return err
		}
		if err := q.AddMember(ctx, sqlcgen.AddMemberParams{HouseholdID: h.ID, UserID: owner, Role: sqlcgen.HouseholdRoleOwner}); err != nil {
			return err
		}
		m = domain.Membership{Household: toHousehold(h), Role: domain.RoleOwner}
		return nil
	})
	return m, err
}

func (s *Store) ListForUser(ctx context.Context, userID uuid.UUID) ([]domain.Membership, error) {
	rows, err := s.q.ListHouseholdsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Membership, len(rows))
	for i, r := range rows {
		out[i] = domain.Membership{Household: domain.Household{ID: r.ID, Name: r.Name}, Role: domain.Role(r.Role)}
	}
	return out, nil
}

func (s *Store) Role(ctx context.Context, householdID, userID uuid.UUID) (domain.Role, error) {
	r, err := s.q.GetMembershipRole(ctx, sqlcgen.GetMembershipRoleParams{HouseholdID: householdID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return domain.Role(r), err
}

func (s *Store) Get(ctx context.Context, id uuid.UUID) (domain.Household, error) {
	h, err := s.q.GetHousehold(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Household{}, domain.ErrNotFound
	}
	return toHousehold(h), err
}

func (s *Store) Members(ctx context.Context, householdID uuid.UUID) ([]domain.Member, error) {
	rows, err := s.q.ListMembers(ctx, householdID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Member, len(rows))
	for i, r := range rows {
		out[i] = domain.Member{
			User: domain.User{ID: r.ID, Name: r.Name, Email: deref(r.Email), AvatarURL: deref(r.AvatarUrl)},
			Role: domain.Role(r.Role),
		}
	}
	return out, nil
}

// HealthCheck reports whether Postgres answers.
type HealthCheck struct{ Pool *pgxpool.Pool }

func (HealthCheck) Name() string                      { return "postgres" }
func (h HealthCheck) Check(ctx context.Context) error { return h.Pool.Ping(ctx) }

func toHousehold(h sqlcgen.Household) domain.Household {
	return domain.Household{ID: h.ID, Name: h.Name, CreatedAt: h.CreatedAt}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
