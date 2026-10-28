package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Shopping is the household's shared shopping list: every member sees, adds and ticks off the same
// items.
type Shopping struct {
	households port.HouseholdRepository
	items      port.ShoppingRepository
}

func NewShopping(households port.HouseholdRepository, items port.ShoppingRepository) *Shopping {
	return &Shopping{households: households, items: items}
}

func (s *Shopping) Items(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.ShoppingItem, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.items.ShoppingItems(ctx, householdID, domain.MaxShoppingItems)
}

func (s *Shopping) Add(ctx context.Context, actor domain.User, householdID uuid.UUID, texts []string, recipe string) ([]domain.ShoppingItem, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	if len(texts) == 0 || len(texts) > domain.MaxShoppingItems {
		return nil, fmt.Errorf("%w: add 1-%d items", domain.ErrInvalid, domain.MaxShoppingItems)
	}
	items := make([]domain.ShoppingItem, len(texts))
	for i, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" || utf8.RuneCountInString(text) > domain.MaxShoppingText {
			return nil, domain.TextError{Max: domain.MaxShoppingText}
		}
		items[i] = domain.ShoppingItem{HouseholdID: householdID, Text: text, Recipe: recipe, AddedBy: &actor.ID}
	}
	return s.items.AddShoppingItems(ctx, items)
}

func (s *Shopping) SetDone(ctx context.Context, actor domain.User, householdID, id uuid.UUID, done bool) (domain.ShoppingItem, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.ShoppingItem{}, err
	}
	return s.items.SetShoppingItemDone(ctx, householdID, id, done)
}

func (s *Shopping) Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	return s.items.DeleteShoppingItem(ctx, householdID, id)
}

func (s *Shopping) ClearDone(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	return s.items.ClearDoneShoppingItems(ctx, householdID)
}
