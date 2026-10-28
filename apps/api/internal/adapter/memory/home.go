package memory

import (
	"context"
	"sync"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// SmartHome is a stand-in for Home Assistant in tests. Set Down to make every call unavailable.
type SmartHome struct {
	mu     sync.Mutex
	scenes []domain.Scene
	lights []domain.Light
	Down   bool
	// Activated lists the scenes turned on, in order.
	Activated []string
}

func NewSmartHome(scenes []domain.Scene, lights []domain.Light) *SmartHome {
	return &SmartHome{scenes: scenes, lights: lights}
}

func (h *SmartHome) Devices(context.Context) ([]domain.Scene, []domain.Light, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Down {
		return nil, nil, domain.ErrUnavailable
	}
	return append([]domain.Scene{}, h.scenes...), append([]domain.Light{}, h.lights...), nil
}

func (h *SmartHome) ActivateScene(_ context.Context, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Down {
		return domain.ErrUnavailable
	}
	h.Activated = append(h.Activated, id)
	return nil
}

func (h *SmartHome) ToggleLight(_ context.Context, id string) (domain.Light, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Down {
		return domain.Light{}, domain.ErrUnavailable
	}
	for i, l := range h.lights {
		if l.ID == id {
			h.lights[i].On = !l.On
			return h.lights[i], nil
		}
	}
	return domain.Light{}, domain.ErrNotFound
}
