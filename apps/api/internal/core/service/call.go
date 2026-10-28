package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

type Call struct {
	households port.HouseholdRepository
	passes     port.SessionStore
	server     port.CallServer
	notify     notifier
}

// NewCall works with a nil server: calls are then switched off.
func NewCall(households port.HouseholdRepository, passes port.SessionStore, server port.CallServer, notify notifier) *Call {
	return &Call{households: households, passes: passes, server: server, notify: notify}
}

func (s *Call) Start(ctx context.Context, actor domain.User, householdID uuid.UUID, invite []uuid.UUID) (domain.Call, error) {
	if err := s.members(ctx, householdID, actor.ID, invite); err != nil {
		return domain.Call{}, err
	}
	if s.server == nil {
		return domain.Call{}, errCallsOff
	}
	c, err := s.join(actor.ID.String(), actor.Name, householdID, uuid.New())
	if err != nil {
		return domain.Call{}, err
	}
	s.ring(ctx, actor, c, invite)
	return c, nil
}

func (s *Call) Join(ctx context.Context, actor domain.User, householdID, callID uuid.UUID) (domain.Call, error) {
	if err := s.members(ctx, householdID, actor.ID, nil); err != nil {
		return domain.Call{}, err
	}
	if s.server == nil {
		return domain.Call{}, errCallsOff
	}
	return s.join(actor.ID.String(), actor.Name, householdID, callID)
}

func (s *Call) Invite(ctx context.Context, actor domain.User, householdID, callID uuid.UUID, userIDs []uuid.UUID) error {
	if err := s.members(ctx, householdID, actor.ID, userIDs); err != nil {
		return err
	}
	if s.server == nil {
		return errCallsOff
	}
	room := callRoom(householdID, callID)
	inCall, err := s.server.Participants(ctx, room)
	if err != nil {
		return err
	}
	var ring []uuid.UUID
	for _, id := range userIDs {
		if !slices.Contains(inCall, id.String()) {
			ring = append(ring, id)
		}
	}
	s.ring(ctx, actor, domain.Call{ID: callID, Room: room}, ring)
	return nil
}

func (s *Call) GuestLink(ctx context.Context, actor domain.User, householdID, callID uuid.UUID) (domain.GuestLink, error) {
	if err := s.members(ctx, householdID, actor.ID, nil); err != nil {
		return domain.GuestLink{}, err
	}
	if s.server == nil {
		return domain.GuestLink{}, errCallsOff
	}
	link := domain.GuestLink{Token: randomID(), ExpiresAt: time.Now().Add(domain.GuestPassTTL).UTC()}
	pass := domain.GuestPass{HouseholdID: householdID, CallID: callID, InvitedBy: actor.Name}
	if err := s.passes.SaveGuestPass(ctx, link.Token, pass); err != nil {
		return domain.GuestLink{}, err
	}
	return link, nil
}

func (s *Call) GuestPass(ctx context.Context, token string) (domain.GuestPass, error) {
	if token == "" {
		return domain.GuestPass{}, domain.ErrNotFound
	}
	return s.passes.GuestPass(ctx, token)
}

func (s *Call) JoinAsGuest(ctx context.Context, token, name string) (domain.Call, error) {
	pass, err := s.GuestPass(ctx, token)
	if err != nil {
		return domain.Call{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > domain.MaxGuestName {
		return domain.Call{}, fmt.Errorf("%w: name must be 1-%d characters", domain.ErrInvalid, domain.MaxGuestName)
	}
	if s.server == nil {
		return domain.Call{}, errCallsOff
	}
	// Guests get an identity of their own each time, so they can never pass for a member.
	return s.join("guest-"+uuid.NewString(), name, pass.HouseholdID, pass.CallID)
}

var errCallsOff = fmt.Errorf("%w: calls are not set up", domain.ErrUnavailable)

// members checks that the actor and everyone in others belong to the household.
func (s *Call) members(ctx context.Context, householdID, actor uuid.UUID, others []uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor); err != nil {
		return err
	}
	for _, id := range others {
		if id == actor {
			return fmt.Errorf("%w: can't call yourself", domain.ErrInvalid)
		}
		if _, err := s.households.Role(ctx, householdID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Call) join(identity, name string, householdID, callID uuid.UUID) (domain.Call, error) {
	room := callRoom(householdID, callID)
	token, err := s.server.Token(identity, name, room)
	if err != nil {
		return domain.Call{}, err
	}
	return domain.Call{ID: callID, URL: s.server.URL(), Token: token, Room: room}, nil
}

// ring tells members someone is calling them; the notification opens the call to answer it.
func (s *Call) ring(ctx context.Context, actor domain.User, c domain.Call, userIDs []uuid.UUID) {
	s.notify.Notify(ctx, userIDs, domain.Notification{
		Key:  "push.call",
		Args: []any{"name", actor.Name},
		URL:  "/call/" + c.ID.String() + "?from=" + actor.ID.String(),
		Tag:  c.Room,
	})
}

// callRoom names a call's room after its household too, so a call id from one household can't
// open a room of another. LiveKit creates the room when the first person joins.
func callRoom(householdID, callID uuid.UUID) string {
	return "call-" + householdID.String() + "-" + callID.String()
}
