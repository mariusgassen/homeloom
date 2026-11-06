package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SaveLocation(ctx context.Context, userID uuid.UUID, l domain.MemberLocation) error {
	return s.q.UpsertMemberLocation(ctx, sqlcgen.UpsertMemberLocationParams{
		UserID: userID, Latitude: l.Latitude, Longitude: l.Longitude, Place: l.Place,
	})
}

func (s *Store) DeleteLocation(ctx context.Context, userID uuid.UUID) error {
	return s.q.DeleteMemberLocation(ctx, userID)
}

func (s *Store) Locations(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]domain.MemberLocation, error) {
	rows, err := s.q.ListMemberLocations(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]domain.MemberLocation, len(rows))
	for _, r := range rows {
		out[r.UserID] = domain.MemberLocation{
			Coords: domain.Coords{Latitude: r.Latitude, Longitude: r.Longitude}, Place: r.Place, UpdatedAt: r.UpdatedAt,
		}
	}
	return out, nil
}
