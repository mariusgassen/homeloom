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

func (s *Call) Join(ctx context.Context, actor domain.User, householdID, with uuid.UUID) (domain.Call, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Call{}, err
	}
	if with == actor.ID {
		return domain.Call{}, fmt.Errorf("%w: can't call yourself", domain.ErrInvalid)
	}
	if _, err := s.households.Role(ctx, householdID, with); err != nil {
		return domain.Call{}, err
	}
	if s.server == nil {
		return domain.Call{}, fmt.Errorf("%w: calls are not set up", domain.ErrUnavailable)
	}
	room := callRoom(householdID, actor.ID, with)
	inCall, err := s.server.Participants(ctx, room)
	if err != nil {
		return domain.Call{}, err
	}
	token, err := s.server.Token(actor, room)
	if err != nil {
		return domain.Call{}, err
	}
	// The other one already in the call means the actor is answering or coming back: no ring.
	if !slices.Contains(inCall, with.String()) {
		s.notify.Notify(ctx, []uuid.UUID{with}, domain.Notification{
			Key:  "push.call",
			Args: []any{"name", actor.Name},
			URL:  "/call?from=" + actor.ID.String(),
			Tag:  room,
		})
	}
	return domain.Call{URL: s.server.URL(), Token: token, Room: room}, nil
}

// callRoom is the room two members share, the same whoever calls; LiveKit creates it when the
// first one joins.
func callRoom(householdID, a, b uuid.UUID) string {
	if b.String() < a.String() {
		a, b = b, a
	}
	return "call-" + householdID.String() + "-" + a.String() + "-" + b.String()
}
