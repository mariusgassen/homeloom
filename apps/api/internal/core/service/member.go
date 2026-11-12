package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Members lets the owner of a household keep its members' details, such as a birthday or a
// picture. Everyone else changes only their own, through Gifts and Avatars.
type Members struct {
	households port.HouseholdRepository
	users      port.UserRepository
	gifts      port.GiftRepository
}

func NewMembers(households port.HouseholdRepository, users port.UserRepository, gifts port.GiftRepository) *Members {
	return &Members{households: households, users: users, gifts: gifts}
}

// owned checks that the actor owns the household and that userID is one of its members.
func (s *Members) owned(ctx context.Context, actor domain.User, householdID, userID uuid.UUID) error {
	role, err := s.households.Role(ctx, householdID, actor.ID)
	if err != nil {
		return err
	}
	if _, err := s.households.Role(ctx, householdID, userID); err != nil {
		return err
	}
	if role != domain.RoleOwner {
		return domain.ErrForbidden
	}
	return nil
}

func (s *Members) SetBirthday(ctx context.Context, actor domain.User, householdID, userID uuid.UUID, b domain.Birthday) error {
	if err := s.owned(ctx, actor, householdID, userID); err != nil {
		return err
	}
	if !b.Valid() {
		return domain.ErrBirthday
	}
	return s.gifts.SetBirthday(ctx, userID, &b)
}

func (s *Members) ClearBirthday(ctx context.Context, actor domain.User, householdID, userID uuid.UUID) error {
	if err := s.owned(ctx, actor, householdID, userID); err != nil {
		return err
	}
	return s.gifts.SetBirthday(ctx, userID, nil)
}

func (s *Members) SetAvatar(ctx context.Context, actor domain.User, householdID, userID uuid.UUID, p domain.Photo) (domain.User, error) {
	if err := s.owned(ctx, actor, householdID, userID); err != nil {
		return domain.User{}, err
	}
	if !validAvatar(p) {
		return domain.User{}, domain.ErrAvatar
	}
	return s.users.SetAvatar(ctx, userID, p)
}

func (s *Members) DeleteAvatar(ctx context.Context, actor domain.User, householdID, userID uuid.UUID) (domain.User, error) {
	if err := s.owned(ctx, actor, householdID, userID); err != nil {
		return domain.User{}, err
	}
	return s.users.DeleteAvatar(ctx, userID)
}
