package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SaveRating(ctx context.Context, r domain.MealRating) error {
	return s.q.UpsertMealRating(ctx, sqlcgen.UpsertMealRatingParams{
		HouseholdID: r.HouseholdID, UserID: r.UserID, RecipeSlug: r.Slug, RecipeName: r.Name, Rating: string(r.Rating),
	})
}

func (s *Store) DeleteRating(ctx context.Context, householdID, userID uuid.UUID, slug string) error {
	return s.q.DeleteMealRating(ctx, sqlcgen.DeleteMealRatingParams{HouseholdID: householdID, UserID: userID, RecipeSlug: slug})
}

func (s *Store) Ratings(ctx context.Context, householdID uuid.UUID) ([]domain.MealRating, error) {
	rows, err := s.q.ListMealRatings(ctx, householdID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MealRating, len(rows))
	for i, r := range rows {
		out[i] = domain.MealRating{
			HouseholdID: r.HouseholdID, UserID: r.UserID, Slug: r.RecipeSlug, Name: r.RecipeName,
			Rating: domain.Rating(r.Rating), RatedAt: r.RatedAt,
		}
	}
	return out, nil
}
