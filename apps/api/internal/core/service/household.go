package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

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
	if name == "" || utf8.RuneCountInString(name) > domain.MaxHouseholdName {
		return domain.Membership{}, fmt.Errorf("%w: name must be 1-%d characters", domain.ErrInvalid, domain.MaxHouseholdName)
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

func (s *Household) Invite(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Invite, error) {
	if _, err := s.repo.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Invite{}, err
	}
	inv := domain.Invite{Code: randomID(), ExpiresAt: time.Now().Add(domain.InviteTTL).UTC()}
	if err := s.repo.CreateInvite(ctx, householdID, actor.ID, hashCode(inv.Code), inv.ExpiresAt); err != nil {
		return domain.Invite{}, err
	}
	return inv, nil
}

func (s *Household) Join(ctx context.Context, actor domain.User, code string) (domain.Membership, error) {
	if code == "" {
		return domain.Membership{}, domain.ErrNotFound
	}
	return s.repo.AcceptInvite(ctx, hashCode(code), actor.ID)
}

// hashCode is what is stored for an invite, so a database dump holds no usable links.
func hashCode(code string) []byte {
	sum := sha256.Sum256([]byte(code))
	return sum[:]
}
