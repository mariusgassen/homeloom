package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) Documents(_ context.Context, householdID, viewer uuid.UUID, limit int) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Document{}
	for _, d := range s.documents {
		if d.HouseholdID == householdID && (d.OwnerID == viewer || d.Shared) {
			out = append(out, d)
		}
	}
	slices.SortStableFunc(out, func(a, b domain.Document) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out[:min(len(out), limit)], nil
}

// document is the index of the document the viewer can see, or -1.
func (s *Store) document(householdID, viewer, id uuid.UUID) int {
	return slices.IndexFunc(s.documents, func(d domain.Document) bool {
		return d.ID == id && d.HouseholdID == householdID && (d.OwnerID == viewer || d.Shared)
	})
}

func (s *Store) GetDocument(_ context.Context, householdID, viewer, id uuid.UUID) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.document(householdID, viewer, id); i >= 0 {
		return s.documents[i], nil
	}
	return domain.Document{}, domain.ErrNotFound
}

func (s *Store) CreateDocument(_ context.Context, d domain.Document) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d.CreatedAt = time.Now()
	s.documents = append(s.documents, d)
	return d, nil
}

func (s *Store) UpdateDocument(_ context.Context, householdID, owner, id uuid.UUID, name string, shared bool) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.document(householdID, owner, id)
	if i < 0 || s.documents[i].OwnerID != owner {
		return domain.Document{}, domain.ErrNotFound
	}
	s.documents[i].Name, s.documents[i].Shared = name, shared
	return s.documents[i], nil
}

func (s *Store) DeleteDocument(_ context.Context, householdID, owner, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.document(householdID, owner, id)
	if i < 0 || s.documents[i].OwnerID != owner {
		return domain.ErrNotFound
	}
	s.documents = slices.Delete(s.documents, i, i+1)
	return nil
}
