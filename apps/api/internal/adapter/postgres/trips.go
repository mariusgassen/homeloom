package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) Trips(ctx context.Context, householdID uuid.UUID, limit int) ([]domain.Trip, error) {
	rows, err := s.q.ListTrips(ctx, sqlcgen.ListTripsParams{HouseholdID: householdID, Lim: int32(min(limit, 1000))}) //nolint:gosec // bounded above
	if err != nil {
		return nil, err
	}
	options, err := s.q.ListTripOptions(ctx, householdID)
	if err != nil {
		return nil, err
	}
	photos, err := s.q.ListTripPhotos(ctx, householdID)
	if err != nil {
		return nil, err
	}
	votes, err := s.q.ListTripVotes(ctx, householdID)
	if err != nil {
		return nil, err
	}
	packing, err := s.q.ListTripPacking(ctx, householdID)
	if err != nil {
		return nil, err
	}
	return assembleTrips(rows, options, photos, votes, packing), nil
}

func (s *Store) Trip(ctx context.Context, householdID, id uuid.UUID) (domain.Trip, error) {
	row, err := s.q.GetTrip(ctx, sqlcgen.GetTripParams{ID: id, HouseholdID: householdID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Trip{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Trip{}, err
	}
	options, err := s.q.ListTripOptionsOf(ctx, id)
	if err != nil {
		return domain.Trip{}, err
	}
	photos, err := s.q.ListTripPhotosOf(ctx, id)
	if err != nil {
		return domain.Trip{}, err
	}
	votes, err := s.q.ListTripVotesOf(ctx, id)
	if err != nil {
		return domain.Trip{}, err
	}
	packing, err := s.q.ListTripPackingOf(ctx, id)
	if err != nil {
		return domain.Trip{}, err
	}
	return assembleTrips([]sqlcgen.Trip{row}, options, photos, votes, packing)[0], nil
}

func (s *Store) CreateTrip(ctx context.Context, t domain.Trip) (domain.Trip, error) {
	row, err := s.q.CreateTrip(ctx, sqlcgen.CreateTripParams{
		HouseholdID: t.HouseholdID, Name: t.Name, Destination: t.Destination, StartOn: t.StartOn, EndOn: t.EndOn,
		Notes: t.Notes, CreatedBy: t.CreatedBy,
	})
	if err != nil {
		return domain.Trip{}, err
	}
	return assembleTrips([]sqlcgen.Trip{row}, nil, nil, nil, nil)[0], nil
}

func (s *Store) UpdateTrip(ctx context.Context, t domain.Trip) (domain.Trip, error) {
	row, err := s.q.UpdateTrip(ctx, sqlcgen.UpdateTripParams{
		Name: t.Name, Destination: t.Destination, StartOn: t.StartOn, EndOn: t.EndOn, Notes: t.Notes,
		ChosenOptionID: t.ChosenOptionID, ID: t.ID, HouseholdID: t.HouseholdID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Trip{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Trip{}, err
	}
	return assembleTrips([]sqlcgen.Trip{row}, nil, nil, nil, nil)[0], nil
}

func (s *Store) DeleteTrip(ctx context.Context, householdID, id uuid.UUID) error {
	n, err := s.q.DeleteTrip(ctx, sqlcgen.DeleteTripParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) CreateTripOption(ctx context.Context, o domain.TripOption) (domain.TripOption, error) {
	row, err := s.q.CreateTripOption(ctx, sqlcgen.CreateTripOptionParams{
		TripID: o.TripID, Title: o.Title, Url: o.URL, Notes: o.Notes, AddedBy: o.AddedBy,
	})
	if err != nil {
		return domain.TripOption{}, err
	}
	return toTripOption(row), nil
}

func (s *Store) UpdateTripOption(ctx context.Context, tripID uuid.UUID, o domain.TripOption) error {
	n, err := s.q.UpdateTripOption(ctx, sqlcgen.UpdateTripOptionParams{ID: o.ID, TripID: tripID, Title: o.Title, Url: o.URL, Notes: o.Notes})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteTripOption(ctx context.Context, tripID, id uuid.UUID) error {
	n, err := s.q.DeleteTripOption(ctx, sqlcgen.DeleteTripOptionParams{ID: id, TripID: tripID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) SetTripVote(ctx context.Context, tripID, userID uuid.UUID, optionID *uuid.UUID) error {
	if optionID == nil {
		return s.q.ClearTripVote(ctx, sqlcgen.ClearTripVoteParams{TripID: tripID, UserID: userID})
	}
	return s.q.SetTripVote(ctx, sqlcgen.SetTripVoteParams{TripID: tripID, UserID: userID, OptionID: *optionID})
}

func (s *Store) CreateTripPhoto(ctx context.Context, p domain.TripPhoto) (domain.TripPhoto, error) {
	row, err := s.q.CreateTripPhoto(ctx, p.OptionID)
	if err != nil {
		return domain.TripPhoto{}, err
	}
	return domain.TripPhoto{ID: row.ID, OptionID: row.OptionID, CreatedAt: row.CreatedAt}, nil
}

func (s *Store) DeleteTripPhoto(ctx context.Context, optionID, id uuid.UUID) error {
	n, err := s.q.DeleteTripPhoto(ctx, sqlcgen.DeleteTripPhotoParams{ID: id, OptionID: optionID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) CreatePackingItem(ctx context.Context, tripID uuid.UUID, text string) (domain.PackingItem, error) {
	row, err := s.q.CreatePackingItem(ctx, sqlcgen.CreatePackingItemParams{TripID: tripID, Text: text})
	if err != nil {
		return domain.PackingItem{}, err
	}
	return toPackingItem(row), nil
}

func (s *Store) UpdatePackingItem(ctx context.Context, tripID, id uuid.UUID, f domain.PackingFields) error {
	n, err := s.q.UpdatePackingItem(ctx, sqlcgen.UpdatePackingItemParams{ID: id, TripID: tripID, Text: f.Text, Done: f.Done})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) DeletePackingItem(ctx context.Context, tripID, id uuid.UUID) error {
	n, err := s.q.DeletePackingItem(ctx, sqlcgen.DeletePackingItemParams{ID: id, TripID: tripID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// assembleTrips puts options, photos, votes and packing items on their trips; each list is in the
// order its query returned, and the trips keep their order.
func assembleTrips(trips []sqlcgen.Trip, options []sqlcgen.TripOption, photos []sqlcgen.TripPhoto, votes []sqlcgen.TripVote, packing []sqlcgen.TripPacking) []domain.Trip {
	out := make([]domain.Trip, len(trips))
	at := make(map[uuid.UUID]int, len(trips))
	for i, r := range trips {
		out[i] = domain.Trip{
			ID: r.ID, HouseholdID: r.HouseholdID, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
			TripFields: domain.TripFields{
				Name: r.Name, Destination: r.Destination, StartOn: r.StartOn, EndOn: r.EndOn, Notes: r.Notes,
				ChosenOptionID: r.ChosenOptionID,
			},
			Options: []domain.TripOption{}, Packing: []domain.PackingItem{}, Votes: []domain.TripVote{},
		}
		at[r.ID] = i
	}
	optionAt := make(map[uuid.UUID][2]int, len(options))
	for _, r := range options {
		i, ok := at[r.TripID]
		if !ok {
			continue
		}
		optionAt[r.ID] = [2]int{i, len(out[i].Options)}
		out[i].Options = append(out[i].Options, toTripOption(r))
	}
	for _, r := range photos {
		if p, ok := optionAt[r.OptionID]; ok {
			o := &out[p[0]].Options[p[1]]
			o.Photos = append(o.Photos, domain.TripPhoto{ID: r.ID, OptionID: r.OptionID, CreatedAt: r.CreatedAt})
		}
	}
	for _, r := range votes {
		if i, ok := at[r.TripID]; ok {
			out[i].Votes = append(out[i].Votes, domain.TripVote{UserID: r.UserID, OptionID: r.OptionID})
		}
	}
	for _, r := range packing {
		if i, ok := at[r.TripID]; ok {
			out[i].Packing = append(out[i].Packing, toPackingItem(r))
		}
	}
	return out
}

func toTripOption(r sqlcgen.TripOption) domain.TripOption {
	return domain.TripOption{
		ID: r.ID, TripID: r.TripID, AddedBy: r.AddedBy, CreatedAt: r.CreatedAt, Photos: []domain.TripPhoto{},
		TripOptionFields: domain.TripOptionFields{Title: r.Title, URL: r.Url, Notes: r.Notes},
	}
}

func toPackingItem(r sqlcgen.TripPacking) domain.PackingItem {
	return domain.PackingItem{ID: r.ID, TripID: r.TripID, Text: r.Text, Done: r.Done, CreatedAt: r.CreatedAt}
}
