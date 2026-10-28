package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Home shows the home room by room and switches scenes and lights in Home Assistant. Each member connects their own Home Assistant
// account, so every switch acts as, and is logged for, the person who made it. Only
// entities Home Assistant lists can be switched, so a caller can't reach anything outside the
// whitelist by guessing an id. Without Home Assistant (smart is nil) the feature is unavailable.
type Home struct {
	households  port.HouseholdRepository
	connections port.HomeConnectionRepository
	sessions    port.SessionStore
	smart       port.SmartHome
}

func NewHome(households port.HouseholdRepository, connections port.HomeConnectionRepository, sessions port.SessionStore, smart port.SmartHome) *Home {
	return &Home{households: households, connections: connections, sessions: sessions, smart: smart}
}

func (s *Home) Connect(ctx context.Context, actor domain.User, householdID uuid.UUID) (string, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return "", err
	}
	state := randomID()
	if err := s.sessions.SaveHomeConnection(ctx, state, domain.PendingHomeConnection{UserID: actor.ID}); err != nil {
		return "", err
	}
	return s.smart.AuthorizeURL(state), nil
}

func (s *Home) CompleteConnect(ctx context.Context, actor domain.User, state, code string) error {
	if s.smart == nil {
		return fmt.Errorf("%w: Home Assistant is not configured", domain.ErrUnavailable)
	}
	if state == "" || code == "" {
		return fmt.Errorf("%w: missing state or code", domain.ErrUnauthenticated)
	}
	pending, err := s.sessions.TakeHomeConnection(ctx, state)
	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("%w: unknown or expired state", domain.ErrUnauthenticated)
	}
	if err != nil {
		return err
	}
	// The sign-in must come back to the person who started it.
	if pending.UserID != actor.ID {
		return fmt.Errorf("%w: started by someone else", domain.ErrUnauthenticated)
	}
	token, err := s.smart.Exchange(ctx, code)
	if err != nil {
		return err
	}
	old, err := s.connections.HomeConnection(ctx, actor.ID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if err := s.connections.SaveHomeConnection(ctx, domain.HomeConnection{UserID: actor.ID, RefreshToken: token}); err != nil {
		return err
	}
	if old.RefreshToken != "" {
		s.revoke(ctx, old.RefreshToken)
	}
	return nil
}

func (s *Home) Disconnect(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return err
	}
	c, err := s.connections.HomeConnection(ctx, actor.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.connections.DeleteHomeConnection(ctx, actor.ID); err != nil {
		return err
	}
	s.revoke(ctx, c.RefreshToken)
	return nil
}

func (s *Home) Get(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Home, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.Home{}, err
	}
	notConnected := domain.Home{Rooms: []domain.Room{}, Devices: domain.Devices{Scenes: []domain.Scene{}, Lights: []domain.Light{}, Readings: []domain.Reading{}}}
	c, err := s.connections.HomeConnection(ctx, actor.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return notConnected, nil
	}
	if err != nil {
		return domain.Home{}, err
	}
	devices, err := s.smart.Devices(ctx, c.RefreshToken)
	if errors.Is(err, domain.ErrUnauthenticated) {
		// Revoked in Home Assistant: forget it, so the app offers to connect again.
		slog.WarnContext(ctx, "home assistant rejected the connection", "err", err)
		if err := s.connections.DeleteHomeConnection(ctx, actor.ID); err != nil {
			return domain.Home{}, err
		}
		return notConnected, nil
	}
	if err != nil {
		return domain.Home{}, err
	}
	layout, err := s.smart.Layout(ctx, c.RefreshToken)
	if err != nil {
		// Without the rooms the home still works, as one list.
		slog.WarnContext(ctx, "home assistant rooms unavailable", "err", err)
		layout = domain.Layout{}
	}
	return arrange(devices, layout), nil
}

// arrange puts each entity in its room and leaves out what Home Assistant hides. Only rooms that
// end up holding something are kept.
func arrange(d domain.Devices, l domain.Layout) domain.Home {
	used := map[string]bool{}
	room := func(id string) string {
		r := l.Areas[id]
		if r != "" {
			used[r] = true
		}
		return r
	}
	h := domain.Home{Connected: true, Rooms: []domain.Room{}, Devices: domain.Devices{Scenes: []domain.Scene{}, Lights: []domain.Light{}, Readings: []domain.Reading{}}}
	for _, x := range d.Scenes {
		if !l.Hidden[x.ID] {
			x.Room = room(x.ID)
			h.Scenes = append(h.Scenes, x)
		}
	}
	for _, x := range d.Lights {
		if !l.Hidden[x.ID] {
			x.Room = room(x.ID)
			h.Lights = append(h.Lights, x)
		}
	}
	for _, x := range d.Readings {
		if !l.Hidden[x.ID] {
			x.Room = room(x.ID)
			h.Readings = append(h.Readings, x)
		}
	}
	for _, r := range l.Rooms {
		if used[r.ID] {
			h.Rooms = append(h.Rooms, r)
		}
	}
	return h
}

func (s *Home) ActivateScene(ctx context.Context, actor domain.User, householdID uuid.UUID, sceneID string) error {
	token, err := s.token(ctx, actor, householdID)
	if err != nil {
		return err
	}
	devices, err := s.smart.Devices(ctx, token)
	if err != nil {
		return unavailable(err)
	}
	for _, sc := range devices.Scenes {
		if sc.ID == sceneID {
			return unavailable(s.smart.ActivateScene(ctx, token, sceneID))
		}
	}
	return domain.ErrNotFound
}

func (s *Home) ToggleLight(ctx context.Context, actor domain.User, householdID uuid.UUID, lightID string) (domain.Light, error) {
	token, err := s.token(ctx, actor, householdID)
	if err != nil {
		return domain.Light{}, err
	}
	devices, err := s.smart.Devices(ctx, token)
	if err != nil {
		return domain.Light{}, unavailable(err)
	}
	for _, l := range devices.Lights {
		if l.ID == lightID {
			l, err := s.smart.ToggleLight(ctx, token, lightID)
			return l, unavailable(err)
		}
	}
	return domain.Light{}, domain.ErrNotFound
}

// token returns the actor's refresh token; without a connection Home Assistant is unavailable.
func (s *Home) token(ctx context.Context, actor domain.User, householdID uuid.UUID) (string, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return "", err
	}
	c, err := s.connections.HomeConnection(ctx, actor.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", fmt.Errorf("%w: Home Assistant is not connected", domain.ErrUnavailable)
	}
	return c.RefreshToken, err
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

func (s *Home) revoke(ctx context.Context, token string) {
	if err := s.smart.Revoke(ctx, token); err != nil {
		slog.WarnContext(ctx, "could not revoke home assistant token", "err", err)
	}
}

// unavailable keeps a rejected token from surfacing as the caller's own authentication failing.
func unavailable(err error) error {
	if errors.Is(err, domain.ErrUnauthenticated) {
		return fmt.Errorf("%w: %v", domain.ErrUnavailable, err) //nolint:errorlint // deliberately not unauthenticated any more
	}
	return err
}
