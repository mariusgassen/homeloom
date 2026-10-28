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

const maxHouseholdName = 100

type Household struct {
	repo port.HouseholdRepository
}

func NewHousehold(repo port.HouseholdRepository) *Household {
	return &Household{repo: repo}
}

func (s *Household) List(ctx context.Context, actor domain.User) ([]domain.Membership, error) {
	return s.repo.ListForUser(ctx, actor.ID)
}

func (s *Household) Create(ctx context.Context, actor domain.User, name string) (domain.Membership, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxHouseholdName {
		return domain.Membership{}, fmt.Errorf("%w: name must be 1-%d characters", domain.ErrInvalid, maxHouseholdName)
	}
	return s.repo.Create(ctx, name, actor.ID)
}

func (s *Household) Get(ctx context.Context, actor domain.User, id uuid.UUID) (domain.HouseholdDetail, error) {
	role, err := s.repo.Role(ctx, id, actor.ID)
	if err != nil {
		return domain.HouseholdDetail{}, err
	}
	h, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.HouseholdDetail{}, err
	}
	members, err := s.repo.Members(ctx, id)
	if err != nil {
		return domain.HouseholdDetail{}, err
	}
	return domain.HouseholdDetail{
		Membership: domain.Membership{Household: h, Role: role},
		Members:    members,
	}, nil
}
