package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// notifyTimeout bounds one round of notifications, across all devices.
const notifyTimeout = 30 * time.Second

// pushHosts are the push services a subscription may point at. The API posts to the endpoint, so
// anything else would let a member make it call arbitrary URLs.
var pushHosts = []string{
	"push.apple.com",            // Safari, including home-screen apps on iOS
	"fcm.googleapis.com",        // Chrome, Edge on Android
	"push.services.mozilla.com", // Firefox
	"notify.windows.com",        // Edge on Windows
	"android.googleapis.com",    // older Chrome
}

type Push struct {
	subs   port.PushSubscriptionRepository
	pusher port.Pusher
	wg     sync.WaitGroup
}

// NewPush works with a nil pusher: push is then switched off and Notify does nothing.
func NewPush(subs port.PushSubscriptionRepository, pusher port.Pusher) *Push {
	return &Push{subs: subs, pusher: pusher}
}

func (s *Push) PublicKey(context.Context) (string, error) {
	if s.pusher == nil {
		return "", fmt.Errorf("%w: push is not set up", domain.ErrUnavailable)
	}
	return s.pusher.PublicKey(), nil
}

func (s *Push) Subscribe(ctx context.Context, actor domain.User, sub domain.PushSubscription) error {
	if s.pusher == nil {
		return fmt.Errorf("%w: push is not set up", domain.ErrUnavailable)
	}
	if !validEndpoint(sub.Endpoint) {
		return fmt.Errorf("%w: not a known push service", domain.ErrInvalid)
	}
	sub.UserID = actor.ID
	return s.subs.SavePushSubscription(ctx, sub)
}

func (s *Push) Unsubscribe(ctx context.Context, actor domain.User, endpoint string) error {
	return s.subs.DeletePushSubscription(ctx, actor.ID, endpoint)
}

// Notify sends n to every device of the given users in the background, so the caller's request
// doesn't wait for push services. Failures are logged, never returned: a notification is a nicety.
func (s *Push) Notify(ctx context.Context, userIDs []uuid.UUID, n domain.Notification) {
	if s.pusher == nil || len(userIDs) == 0 {
		return
	}
	s.wg.Go(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), notifyTimeout)
		defer cancel()
		subs, err := s.subs.PushSubscriptions(ctx, userIDs)
		if err != nil {
			slog.WarnContext(ctx, "push: list subscriptions", "err", err)
			return
		}
		for _, sub := range subs {
			err := s.pusher.Send(ctx, sub, n)
			switch {
			case errors.Is(err, domain.ErrNotFound):
				if err := s.subs.DeletePushEndpoint(ctx, sub.Endpoint); err != nil {
					slog.WarnContext(ctx, "push: drop expired subscription", "err", err)
				}
			case err != nil:
				slog.WarnContext(ctx, "push: send", "err", err, "key", n.Key)
			}
		}
	})
}

// Wait blocks until every notification under way has been sent or given up.
func (s *Push) Wait() { s.wg.Wait() }

func validEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Port() != "" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range pushHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}
