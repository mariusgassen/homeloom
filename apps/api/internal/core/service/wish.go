package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Wishes is the household's wishlist of things to watch, read or visit. Every member sees, adds,
// ticks off and deletes the same wishes.
type Wishes struct {
	households port.HouseholdRepository
	wishes     port.WishRepository
}

func NewWishes(households port.HouseholdRepository, wishes port.WishRepository) *Wishes {
	return &Wishes{households: households, wishes: wishes}
}

func (s *Wishes) List(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.WishKind) ([]domain.Wish, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	if kind != "" && !kind.Valid() {
		return nil, domain.ErrWishKind
	}
	return s.wishes.Wishes(ctx, householdID, kind, domain.MaxWishes)
}

func (s *Wishes) Add(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.WishKind, text string) (domain.Wish, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Wish{}, err
	}
	if !kind.Valid() {
		return domain.Wish{}, domain.ErrWishKind
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > domain.MaxWishText {
		return domain.Wish{}, domain.TextError{Max: domain.MaxWishText}
	}
	return s.wishes.AddWish(ctx, domain.Wish{HouseholdID: householdID, Kind: kind, Text: text, AddedBy: &actor.ID})
}

func (s *Wishes) SetDone(ctx context.Context, actor domain.User, householdID, id uuid.UUID, done bool) (domain.Wish, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Wish{}, err
	}
	return s.wishes.SetWishDone(ctx, householdID, id, done)
}

func (s *Wishes) Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	return s.wishes.DeleteWish(ctx, householdID, id)
}
