package memory

import (
	"context"
	"sync"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// Sessions is an in-process SessionStore. Entries never expire.
type Sessions struct {
	mu       sync.Mutex
	logins   map[string]domain.PendingLogin
	sessions map[string]domain.Session
	homes    map[string]domain.PendingHomeConnection
}

func NewSessions() *Sessions {
	return &Sessions{logins: map[string]domain.PendingLogin{}, sessions: map[string]domain.Session{}, homes: map[string]domain.PendingHomeConnection{}}
}

func (s *Sessions) SaveLogin(_ context.Context, state string, l domain.PendingLogin) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logins[state] = l
	return nil
}

func (s *Sessions) TakeLogin(_ context.Context, state string) (domain.PendingLogin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.logins[state]
	if !ok {
		return l, domain.ErrNotFound
	}
	delete(s.logins, state)
	return l, nil
}

func (s *Sessions) Save(_ context.Context, id string, sess domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = sess
	return nil
}

func (s *Sessions) Get(_ context.Context, id string) (domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return sess, domain.ErrNotFound
	}
	return sess, nil
}

func (s *Sessions) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *Sessions) SaveHomeConnection(_ context.Context, state string, p domain.PendingHomeConnection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.homes[state] = p
	return nil
}

func (s *Sessions) TakeHomeConnection(_ context.Context, state string) (domain.PendingHomeConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.homes[state]
	if !ok {
		return p, domain.ErrNotFound
	}
	delete(s.homes, state)
	return p, nil
}
