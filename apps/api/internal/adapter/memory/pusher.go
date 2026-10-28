package memory

import (
	"context"
	"sync"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// Pushed is one notification a Pusher would have sent.
type Pushed struct {
	Subscription domain.PushSubscription
	Notification domain.Notification
}

// Pusher records notifications instead of sending them. Endpoints in Gone answer as expired.
type Pusher struct {
	Gone map[string]bool

	mu   sync.Mutex
	sent []Pushed
}

func (p *Pusher) PublicKey() string { return "test-public-key" }

func (p *Pusher) Send(_ context.Context, sub domain.PushSubscription, n domain.Notification) error {
	if p.Gone[sub.Endpoint] {
		return domain.ErrNotFound
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, Pushed{sub, n})
	return nil
}

func (p *Pusher) Sent() []Pushed {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Pushed(nil), p.sent...)
}
