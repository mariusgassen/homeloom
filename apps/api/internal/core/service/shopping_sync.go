package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Shopping) SyncItems(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.ShoppingItem, error) {
	return s.Items(ctx, actor, householdID)
}

func (s *Shopping) SyncItem(ctx context.Context, actor domain.User, householdID uuid.UUID, name string) (domain.ShoppingItem, error) {
	items, err := s.Items(ctx, actor, householdID)
	if err != nil {
		return domain.ShoppingItem{}, err
	}
	for _, it := range items {
		if it.SyncName() == name {
			return it, nil
		}
	}
	return domain.ShoppingItem{}, domain.ErrNotFound
}

func (s *Shopping) SyncPut(ctx context.Context, actor domain.User, householdID uuid.UUID, name, uid, text string, done bool) (domain.ShoppingItem, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > domain.MaxShoppingText {
		return domain.ShoppingItem{}, domain.TextError{Max: domain.MaxShoppingText}
	}
	it, err := s.SyncItem(ctx, actor, householdID, name)
	switch {
	case err == nil:
		if it.Text != text {
			if it, err = s.items.SetShoppingItemText(ctx, householdID, it.ID, text); err != nil {
				return domain.ShoppingItem{}, err
			}
		}
		if (it.DoneAt != nil) != done {
			return s.items.SetShoppingItemDone(ctx, householdID, it.ID, done)
		}
		return it, nil
	case !errors.Is(err, domain.ErrNotFound):
		return domain.ShoppingItem{}, err
	}
	if name == "" || strings.Contains(name, "/") || utf8.RuneCountInString(name) > domain.MaxICalName {
		return domain.ShoppingItem{}, fmt.Errorf("%w: name must be 1-%d characters without a slash", domain.ErrInvalid, domain.MaxICalName)
	}
	added, err := s.AddItems(ctx, actor, householdID, []domain.ShoppingItem{{Text: text, ICalName: name, ICalUID: uid}})
	if err != nil {
		return domain.ShoppingItem{}, err
	}
	if done {
		return s.items.SetShoppingItemDone(ctx, householdID, added[0].ID, true)
	}
	return added[0], nil
}

func (s *Shopping) SyncDelete(ctx context.Context, actor domain.User, householdID uuid.UUID, name string) error {
	it, err := s.SyncItem(ctx, actor, householdID, name)
	if err != nil {
		return err
	}
	return s.items.DeleteShoppingItem(ctx, householdID, it.ID)
}
