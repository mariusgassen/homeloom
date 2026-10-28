// Package memory is an in-process implementation of the repository ports, used in tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

type membership struct {
	householdID, userID uuid.UUID
	role                domain.Role
}

type invite struct {
	householdID uuid.UUID
	expiresAt   time.Time
	used        bool
}

type Store struct {
	mu          sync.Mutex
	usersBySub  map[string]domain.User
	users       map[uuid.UUID]domain.User
	households  map[uuid.UUID]domain.Household
	memberships []membership
	invites     map[string]*invite
	questions   []*domain.Question

	homeConnections   map[uuid.UUID]domain.HomeConnection
	pushSubscriptions []domain.PushSubscription
}

func NewStore() *Store {
	return &Store{
		usersBySub: map[string]domain.User{},
		users:      map[uuid.UUID]domain.User{},
		households: map[uuid.UUID]domain.Household{},
		invites:    map[string]*invite{},
	}
}

func (s *Store) UpsertByIdentity(_ context.Context, id domain.Identity) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.usersBySub[id.Subject]
	if !ok {
		u.ID = uuid.New()
	}
	u.Name, u.Email, u.AvatarURL = id.Name, id.Email, id.AvatarURL
	s.usersBySub[id.Subject] = u
	s.users[u.ID] = u
	return u, nil
}

func (s *Store) Create(_ context.Context, name string, owner uuid.UUID) (domain.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := domain.Household{ID: uuid.New(), Name: name, CreatedAt: time.Now()}
	s.households[h.ID] = h
	s.memberships = append(s.memberships, membership{h.ID, owner, domain.RoleOwner})
	return domain.Membership{Household: h, Role: domain.RoleOwner}, nil
}

// AddMember is a test helper that skips the invite.
func (s *Store) AddMember(householdID, userID uuid.UUID, role domain.Role) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memberships = append(s.memberships, membership{householdID, userID, role})
}

// ExpireInvites is a test helper: it makes every invite expired.
func (s *Store) ExpireInvites() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inv := range s.invites {
		inv.expiresAt = time.Time{}
	}
}

func (s *Store) ListForUser(_ context.Context, userID uuid.UUID) ([]domain.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Membership{}
	for _, m := range s.memberships {
		if m.userID == userID {
			out = append(out, domain.Membership{Household: s.households[m.householdID], Role: m.role})
		}
	}
	return out, nil
}

func (s *Store) Role(_ context.Context, householdID, userID uuid.UUID) (domain.Role, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.memberships {
		if m.householdID == householdID && m.userID == userID {
			return m.role, nil
		}
	}
	return "", domain.ErrNotFound
}

func (s *Store) Get(_ context.Context, id uuid.UUID) (domain.Household, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.households[id]
	if !ok {
		return domain.Household{}, domain.ErrNotFound
	}
	return h, nil
}

func (s *Store) Members(_ context.Context, householdID uuid.UUID) ([]domain.Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Member{}
	for _, m := range s.memberships {
		if m.householdID == householdID {
			out = append(out, domain.Member{User: s.users[m.userID], Role: m.role})
		}
	}
	return out, nil
}
