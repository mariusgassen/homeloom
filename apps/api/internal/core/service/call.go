package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

type Call struct {
	households port.HouseholdRepository
	server     port.CallServer
	notify     notifier
}

// NewCall works with a nil server: calls are then switched off.
func NewCall(households port.HouseholdRepository, server port.CallServer, notify notifier) *Call {
	return &Call{households: households, server: server, notify: notify}
}

func (s *Call) Join(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Call, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Call{}, err
	}
	if s.server == nil {
		return domain.Call{}, fmt.Errorf("%w: calls are not set up", domain.ErrUnavailable)
	}
	room := callRoom(householdID)
	inCall, err := s.server.Participants(ctx, room)
	if err != nil {
		return domain.Call{}, err
	}
	members, err := s.households.Members(ctx, householdID)
	if err != nil {
		return domain.Call{}, err
	}
	token, err := s.server.Token(actor, room)
	if err != nil {
		return domain.Call{}, err
	}

	// Someone else already in the call means the actor is answering or coming back: nobody rings.
	var others []uuid.UUID
	for _, m := range members {
		if m.User.ID == actor.ID {
			continue
		}
		if slices.Contains(inCall, m.User.ID.String()) {
			others = nil
			break
		}
		others = append(others, m.User.ID)
	}
	s.notify.Notify(ctx, others, domain.Notification{
		Key:  "push.call",
		Args: []any{"name", actor.Name},
		URL:  "/call?from=" + actor.ID.String(),
		Tag:  room,
	})
	return domain.Call{URL: s.server.URL(), Token: token, Room: room}, nil
}

// callRoom is the household's one call room; LiveKit creates it when the first person joins.
func callRoom(householdID uuid.UUID) string { return "household-" + householdID.String() }
