package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SaveMeal(ctx context.Context, m domain.PlannedMeal) error {
	return s.q.UpsertMeal(ctx, sqlcgen.UpsertMealParams{
		HouseholdID: m.HouseholdID, Day: m.Day, RecipeSlug: m.Recipe.Slug, RecipeName: m.Recipe.Name,
		RecipeImage: m.Recipe.Image, TotalTime: m.Recipe.TotalTime, PlannedBy: m.PlannedBy,
	})
}

func (s *Store) Meals(ctx context.Context, householdID uuid.UUID, from, to time.Time) ([]domain.PlannedMeal, error) {
	rows, err := s.q.ListMeals(ctx, sqlcgen.ListMealsParams{HouseholdID: householdID, FromDay: from, ToDay: to})
	if err != nil {
		return nil, err
	}
	out := make([]domain.PlannedMeal, len(rows))
	for i, r := range rows {
		out[i] = domain.PlannedMeal{
			HouseholdID: r.HouseholdID, Day: r.Day, PlannedBy: r.PlannedBy,
			Recipe: domain.RecipeSummary{Slug: r.RecipeSlug, Name: r.RecipeName, Image: r.RecipeImage, TotalTime: r.TotalTime},
		}
	}
	return out, nil
}

func (s *Store) DeleteMeal(ctx context.Context, householdID uuid.UUID, day time.Time) error {
	return s.q.DeleteMeal(ctx, sqlcgen.DeleteMealParams{HouseholdID: householdID, Day: day})
}
