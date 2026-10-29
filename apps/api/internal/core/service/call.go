package service

import (
	"context"
	"encoding/json"
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
	agent      string
	keys       agentKeys
	voice      *Voice
}

// agentKeys mints the keys the voice agent reaches Homeloom's tools with; *AgentAccess is the real one.
type agentKeys interface {
	Mint(ctx context.Context, actor domain.User, householdID uuid.UUID, ttl time.Duration) (string, error)
}

// voiceCallTTL is how long the voice agent's key works, and so the longest call with Homeloom.
const voiceCallTTL = 2 * time.Hour

// NewCall works with a nil server: calls are then switched off.
func NewCall(households port.HouseholdRepository, passes port.SessionStore, server port.CallServer, notify notifier) *Call {
	return &Call{households: households, passes: passes, server: server, notify: notify}
}

// WithVoiceAgent lets members talk to the voice agent registered as agent, which reaches
// Homeloom's tools with a key from keys.
func (s *Call) WithVoiceAgent(agent string, keys agentKeys) *Call {
	s.agent, s.keys = agent, keys
	return s
}

// WithVoice makes Homeloom sound the way each caller set it up; without it, it uses the defaults.
func (s *Call) WithVoice(v *Voice) *Call {
	s.voice = v
	return s
}

// VoiceCall is the metadata the voice agent gets with each call: the key it acts as the caller
// with, and how to talk to them.
type VoiceCall struct {
	Key   string      `json:"key"`
	Lang  string      `json:"lang"`
	Name  string      `json:"name"`
	Voice VoiceOfCall `json:"voice"`
}

// VoiceOfCall is how the agent speaks: a voice of the speech model, its speed and the
// instructions for its tone.
type VoiceOfCall struct {
	Voice        string  `json:"voice"`
	Speed        float64 `json:"speed"`
	Instructions string  `json:"instructions"`
}

func (s *Call) TalkToHomeloom(ctx context.Context, actor domain.User, householdID uuid.UUID, lang string) (domain.Call, error) {
	if err := s.members(ctx, householdID, actor.ID, nil); err != nil {
		return domain.Call{}, err
	}
	if s.server == nil || s.agent == "" {
		return domain.Call{}, fmt.Errorf("%w: talking to Homeloom is not set up", domain.ErrUnavailable)
	}
	// The room is the actor's alone: no one else can join it, so the agent's answers stay theirs.
	id := uuid.New()
	room := "homeloom-" + householdID.String() + "-" + actor.ID.String() + "-" + id.String()
	key, err := s.keys.Mint(ctx, actor, householdID, voiceCallTTL)
	if err != nil {
		return domain.Call{}, err
	}
	v := s.voice.forCall(ctx, actor)
	meta, err := json.Marshal(VoiceCall{Key: key, Lang: lang, Name: actor.Name, Voice: VoiceOfCall{Voice: v.Voice, Speed: v.Speed, Instructions: v.Tone.Instructions()}})
	if err != nil {
		return domain.Call{}, err
	}
	if err := s.server.Dispatch(ctx, room, s.agent, string(meta)); err != nil {
		return domain.Call{}, err
	}
	token, err := s.server.Token(actor.ID.String(), actor.Name, room)
	if err != nil {
		return domain.Call{}, err
	}
	return domain.Call{ID: id, URL: s.server.URL(), Token: token, Room: room}, nil
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

func (s *Call) JoinAsGuest(ctx context.Context, token, device, name string) (domain.Call, error) {
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
	if device == "" {
		return domain.Call{}, fmt.Errorf("%w: device missing", domain.ErrInvalid)
	}
	// A link is for one guest: a forwarded one doesn't work, but the guest can come back.
	if err := s.passes.ClaimGuestPass(ctx, token, device); err != nil {
		return domain.Call{}, err
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
