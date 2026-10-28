package memory

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// MediaLibrary is a stand-in for Plex in tests. A sign-in whose pin is listed in Approved gives
// the token "token-<pin>"; others are refused, as are tokens in SignedOut. Set Down to make every
// call unavailable.
type MediaLibrary struct {
	mu        sync.Mutex
	libraries []domain.Library
	titles    map[string][]domain.Title
	pins      int
	Down      bool
	Approved  map[string]bool
	SignedOut []string
}

// NewMediaLibrary holds the libraries, and titles by library id.
func NewMediaLibrary(libraries []domain.Library, titles map[string][]domain.Title) *MediaLibrary {
	return &MediaLibrary{libraries: libraries, titles: titles, Approved: map[string]bool{}}
}

// StartSignIn approves every sign-in unless Approved already says otherwise for its pin.
func (m *MediaLibrary) StartSignIn(_ context.Context, state string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Down {
		return "", "", domain.ErrUnavailable
	}
	m.pins++
	pin := "pin-" + strconv.Itoa(m.pins)
	if _, ok := m.Approved[pin]; !ok {
		m.Approved[pin] = true
	}
	return pin, "https://plex.test/auth?state=" + state, nil
}

func (m *MediaLibrary) FinishSignIn(_ context.Context, pin string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.Approved[pin] {
		return "", domain.ErrUnauthenticated
	}
	return "token-" + pin, nil
}

func (m *MediaLibrary) Libraries(_ context.Context, token string) ([]domain.Library, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.check(token); err != nil {
		return nil, err
	}
	return append([]domain.Library{}, m.libraries...), nil
}

func (m *MediaLibrary) Titles(_ context.Context, token, libraryID string) ([]domain.Title, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.check(token); err != nil {
		return nil, err
	}
	titles, ok := m.titles[libraryID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return append([]domain.Title{}, titles...), nil
}

func (m *MediaLibrary) Poster(_ context.Context, token, titleID string) (domain.Poster, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.check(token); err != nil {
		return domain.Poster{}, err
	}
	for _, ts := range m.titles {
		for _, t := range ts {
			if t.ID == titleID {
				return domain.Poster{ContentType: "image/jpeg", Body: []byte("poster " + t.Name)}, nil
			}
		}
	}
	return domain.Poster{}, domain.ErrNotFound
}

// check must be called with mu held.
func (m *MediaLibrary) check(token string) error {
	if m.Down {
		return domain.ErrUnavailable
	}
	if !strings.HasPrefix(token, "token-") {
		return domain.ErrUnauthenticated
	}
	if slices.Contains(m.SignedOut, token) {
		return domain.ErrUnauthenticated
	}
	return nil
}

func (s *Store) SaveWatchConnection(_ context.Context, c domain.WatchConnection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watchConnections == nil {
		s.watchConnections = map[uuid.UUID]domain.WatchConnection{}
	}
	s.watchConnections[c.UserID] = c
	return nil
}

func (s *Store) WatchConnection(_ context.Context, userID uuid.UUID) (domain.WatchConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.watchConnections[userID]
	if !ok {
		return domain.WatchConnection{}, domain.ErrNotFound
	}
	return c, nil
}

func (s *Store) DeleteWatchConnection(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.watchConnections, userID)
	return nil
}
