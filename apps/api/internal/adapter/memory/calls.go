package memory

import (
	"context"
	"sync"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// CallServer is a stand-in for LiveKit in tests: tokens are "<identity>/<name>@<room>", and InRoom says
// who is in each room. Set Down to make it unreachable.
type CallServer struct {
	mu     sync.Mutex
	InRoom map[string][]string
	// Dispatched holds the agent dispatches, as "<agent>@<room>" -> metadata.
	Dispatched map[string]string
	Down       bool
}

func NewCallServer() *CallServer {
	return &CallServer{InRoom: map[string][]string{}, Dispatched: map[string]string{}}
}

func (s *CallServer) URL() string { return "wss://calls.example" }

func (s *CallServer) Token(identity, name, room string) (string, error) {
	return identity + "/" + name + "@" + room, nil
}

func (s *CallServer) Participants(_ context.Context, room string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Down {
		return nil, domain.ErrUnavailable
	}
	return append([]string(nil), s.InRoom[room]...), nil
}

// Enter puts the user in the room, as if they had connected.
func (s *CallServer) Enter(room, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InRoom[room] = append(s.InRoom[room], userID)
}

func (s *CallServer) Dispatch(_ context.Context, room, agent, metadata string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Down {
		return domain.ErrUnavailable
	}
	s.Dispatched[agent+"@"+room] = metadata
	return nil
}
