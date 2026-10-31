package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Shopping is the household's shared shopping list: every member sees, adds and ticks off the same
// items. Each item goes to an aisle: where the household put the same thing before, else where
// the grocer (Claude) says, else "other".
type Shopping struct {
	households port.HouseholdRepository
	items      port.ShoppingRepository
	grocer     port.Grocer // nil: unknown things go to "other"
}

func NewShopping(households port.HouseholdRepository, items port.ShoppingRepository, grocer port.Grocer) *Shopping {
	return &Shopping{households: households, items: items, grocer: grocer}
}

func (s *Shopping) Items(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.ShoppingItem, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.items.ShoppingItems(ctx, householdID, domain.MaxShoppingItems)
}

func (s *Shopping) Add(ctx context.Context, actor domain.User, householdID uuid.UUID, texts []string, recipe string) ([]domain.ShoppingItem, error) {
	items := make([]domain.ShoppingItem, len(texts))
	for i, text := range texts {
		items[i] = domain.ShoppingItem{Text: text, Recipe: recipe}
	}
	return s.AddItems(ctx, actor, householdID, items)
}

func (s *Shopping) AddItems(ctx context.Context, actor domain.User, householdID uuid.UUID, drafts []domain.ShoppingItem) ([]domain.ShoppingItem, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	if len(drafts) == 0 || len(drafts) > domain.MaxShoppingItems {
		return nil, fmt.Errorf("%w: add 1-%d items", domain.ErrInvalid, domain.MaxShoppingItems)
	}
	items := make([]domain.ShoppingItem, len(drafts))
	for i, d := range drafts {
		text := strings.TrimSpace(d.Text)
		if text == "" || utf8.RuneCountInString(text) > domain.MaxShoppingText {
			return nil, domain.TextError{Max: domain.MaxShoppingText}
		}
		items[i] = domain.ShoppingItem{HouseholdID: householdID, Text: text, Recipe: d.Recipe, AddedBy: &actor.ID, Aisle: domain.AisleOther}
	}
	s.sort(ctx, householdID, items)
	return s.items.AddShoppingItems(ctx, items)
}

// sort sets each item's aisle. Not knowing one is never an error: the item goes to "other".
func (s *Shopping) sort(ctx context.Context, householdID uuid.UUID, items []domain.ShoppingItem) {
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = domain.AisleKey(it.Text)
	}
	known, err := s.items.KnownAisles(ctx, householdID, keys)
	if err != nil {
		slog.WarnContext(ctx, "reading shopping aisles failed", "err", err)
	}
	var unknown []string
	for i, k := range keys {
		if a, ok := known[k]; ok {
			items[i].Aisle = a
		} else if !slices.Contains(unknown, k) {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 || s.grocer == nil {
		return
	}
	aisles, err := s.grocer.Aisles(ctx, unknown)
	if err != nil || len(aisles) != len(unknown) {
		slog.WarnContext(ctx, "sorting shopping items failed", "err", err, "asked", len(unknown), "answered", len(aisles))
		return
	}
	learned := make(map[string]domain.Aisle, len(unknown))
	for i, k := range unknown {
		if aisles[i].Valid() {
			learned[k] = aisles[i]
		}
	}
	for i, k := range keys {
		if a, ok := learned[k]; ok {
			items[i].Aisle = a
		}
	}
	if err := s.items.RememberAisles(ctx, householdID, learned); err != nil {
		slog.WarnContext(ctx, "remembering shopping aisles failed", "err", err)
	}
}

func (s *Shopping) SetAisle(ctx context.Context, actor domain.User, householdID, id uuid.UUID, aisle domain.Aisle) (domain.ShoppingItem, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.ShoppingItem{}, err
	}
	if !aisle.Valid() {
		return domain.ShoppingItem{}, fmt.Errorf("%w: unknown aisle %q", domain.ErrInvalid, aisle)
	}
	it, err := s.items.SetShoppingItemAisle(ctx, householdID, id, aisle)
	if err != nil {
		return domain.ShoppingItem{}, err
	}
	if err := s.items.RememberAisles(ctx, householdID, map[string]domain.Aisle{domain.AisleKey(it.Text): aisle}); err != nil {
		return domain.ShoppingItem{}, err
	}
	return it, nil
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

func (s *Shopping) Suggestions(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.TitleSuggestion, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.items.ShoppingSuggestions(ctx, householdID, domain.MaxSuggestions)
}
