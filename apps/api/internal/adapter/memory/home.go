package memory

import (
	"context"
	"slices"
	"sync"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// SmartHome is a stand-in for Home Assistant in tests. Signing in with the code "good" gives
// the refresh token "refresh-good"; revoked tokens are refused. Set Down to make every call
// unavailable.
type SmartHome struct {
	mu     sync.Mutex
	scenes []domain.Scene
	lights []domain.Light
	Down   bool
	// Activated lists the scenes turned on, in order; Revoked the refresh tokens revoked.
	Activated []string
	Revoked   []string
}

func NewSmartHome(scenes []domain.Scene, lights []domain.Light) *SmartHome {
	return &SmartHome{scenes: scenes, lights: lights}
}

func (h *SmartHome) AuthorizeURL(state string) string {
	return "https://ha.test/auth/authorize?state=" + state
}

func (h *SmartHome) Exchange(_ context.Context, code string) (string, error) {
	if code != "good" {
		return "", domain.ErrUnauthenticated
	}
	return "refresh-" + code, nil
}

func (h *SmartHome) Revoke(_ context.Context, refreshToken string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Revoked = append(h.Revoked, refreshToken)
	return nil
}

// check must be called with mu held.
func (h *SmartHome) check(refreshToken string) error {
	if h.Down {
		return domain.ErrUnavailable
	}
	if refreshToken == "" || slices.Contains(h.Revoked, refreshToken) {
		return domain.ErrUnauthenticated
	}
	return nil
}

func (h *SmartHome) Devices(_ context.Context, refreshToken string) ([]domain.Scene, []domain.Light, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(refreshToken); err != nil {
		return nil, nil, err
	}
	return append([]domain.Scene{}, h.scenes...), append([]domain.Light{}, h.lights...), nil
}

func (h *SmartHome) ActivateScene(_ context.Context, refreshToken, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(refreshToken); err != nil {
		return err
	}
	h.Activated = append(h.Activated, id)
	return nil
}

func (h *SmartHome) ToggleLight(_ context.Context, refreshToken, id string) (domain.Light, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(refreshToken); err != nil {
		return domain.Light{}, err
	}
	for i, l := range h.lights {
		if l.ID == id {
			h.lights[i].On = !l.On
			return h.lights[i], nil
		}
	}
	return domain.Light{}, domain.ErrNotFound
}
