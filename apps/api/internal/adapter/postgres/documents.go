package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) Documents(ctx context.Context, householdID, viewer uuid.UUID, limit int) ([]domain.Document, error) {
	rows, err := s.q.ListDocuments(ctx, sqlcgen.ListDocumentsParams{
		HouseholdID: householdID, Viewer: viewer, Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Document, len(rows))
	for i, r := range rows {
		out[i] = toDocument(r)
	}
	return out, nil
}

func (s *Store) GetDocument(ctx context.Context, householdID, viewer, id uuid.UUID) (domain.Document, error) {
	row, err := s.q.GetDocument(ctx, sqlcgen.GetDocumentParams{ID: id, HouseholdID: householdID, Viewer: viewer})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Document{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Document{}, err
	}
	return toDocument(row), nil
}

func (s *Store) CreateDocument(ctx context.Context, d domain.Document) (domain.Document, error) {
	row, err := s.q.CreateDocument(ctx, sqlcgen.CreateDocumentParams{
		ID: d.ID, HouseholdID: d.HouseholdID, OwnerID: d.OwnerID, Name: d.Name, ContentType: d.ContentType, Size: d.Size, Shared: d.Shared,
	})
	if err != nil {
		return domain.Document{}, err
	}
	return toDocument(row), nil
}

func (s *Store) UpdateDocument(ctx context.Context, householdID, owner, id uuid.UUID, name string, shared bool) (domain.Document, error) {
	row, err := s.q.UpdateDocument(ctx, sqlcgen.UpdateDocumentParams{Name: name, Shared: shared, ID: id, HouseholdID: householdID, OwnerID: owner})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Document{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Document{}, err
	}
	return toDocument(row), nil
}

func (s *Store) DeleteDocument(ctx context.Context, householdID, owner, id uuid.UUID) error {
	n, err := s.q.DeleteDocument(ctx, sqlcgen.DeleteDocumentParams{ID: id, HouseholdID: householdID, OwnerID: owner})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func toDocument(r sqlcgen.Document) domain.Document {
	return domain.Document{
		ID: r.ID, HouseholdID: r.HouseholdID, OwnerID: r.OwnerID, Name: r.Name,
		ContentType: r.ContentType, Size: r.Size, Shared: r.Shared, CreatedAt: r.CreatedAt,
	}
}
