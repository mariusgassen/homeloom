package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Home switches scenes and lights in Home Assistant. Only entities the smart home lists can be
// switched, so a caller can't reach anything outside the whitelist by guessing an id. Without
// Home Assistant (smart is nil) the feature is unavailable.
type Home struct {
	households port.HouseholdRepository
	smart      port.SmartHome
}

func NewHome(households port.HouseholdRepository, smart port.SmartHome) *Home {
	return &Home{households: households, smart: smart}
}

func (s *Home) Get(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Home, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.Home{}, err
	}
	scenes, lights, err := s.smart.Devices(ctx)
	if err != nil {
		return domain.Home{}, err
	}
	return domain.Home{Scenes: scenes, Lights: lights}, nil
}

func (s *Home) ActivateScene(ctx context.Context, actor domain.User, householdID uuid.UUID, sceneID string) error {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return err
	}
	scenes, _, err := s.smart.Devices(ctx)
	if err != nil {
		return err
	}
	for _, sc := range scenes {
		if sc.ID == sceneID {
			return s.smart.ActivateScene(ctx, sceneID)
		}
	}
	return domain.ErrNotFound
}

func (s *Home) ToggleLight(ctx context.Context, actor domain.User, householdID uuid.UUID, lightID string) (domain.Light, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.Light{}, err
	}
	_, lights, err := s.smart.Devices(ctx)
	if err != nil {
		return domain.Light{}, err
	}
	for _, l := range lights {
		if l.ID == lightID {
			return s.smart.ToggleLight(ctx, lightID)
		}
	}
	return domain.Light{}, domain.ErrNotFound
}

// ready checks membership first, so non-members learn nothing about whether Home Assistant is set up.
func (s *Home) ready(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	if s.smart == nil {
		return fmt.Errorf("%w: Home Assistant is not configured", domain.ErrUnavailable)
	}
	return nil
}
