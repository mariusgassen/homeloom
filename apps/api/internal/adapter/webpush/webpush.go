// Package webpush delivers notifications through the browsers' push services, signed with VAPID.
package webpush

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

const (
	// sendTimeout bounds the call to one push service.
	sendTimeout = 10 * time.Second
	// ttl is how long a push service keeps a notification for a device that is offline.
	ttl = 24 * 60 * 60
)

type Pusher struct {
	publicKey, privateKey, subject string
	client                         *http.Client
}

// New takes the VAPID key pair as GenerateKeys writes it. subject identifies the sender to the
// push services: an https URL or an e-mail address.
func New(publicKey, privateKey, subject string) (*Pusher, error) {
	if pub, err := base64.RawURLEncoding.DecodeString(publicKey); err != nil || len(pub) != 65 {
		return nil, errors.New("VAPID_PUBLIC_KEY: want an uncompressed P-256 key, base64url without padding")
	}
	if priv, err := base64.RawURLEncoding.DecodeString(privateKey); err != nil || len(priv) != 32 {
		return nil, errors.New("VAPID_PRIVATE_KEY: want a P-256 key, base64url without padding")
	}
	return &Pusher{publicKey: publicKey, privateKey: privateKey, subject: subject, client: &http.Client{Timeout: sendTimeout}}, nil
}

// GenerateKeys returns a new VAPID key pair. Changing the keys means every device subscribes again.
func GenerateKeys() (publicKey, privateKey string, err error) {
	privateKey, publicKey, err = webpush.GenerateVAPIDKeys()
	return publicKey, privateKey, err
}

func (p *Pusher) PublicKey() string { return p.publicKey }

// message is what the service worker's push handler reads.
type message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

func render(sub domain.PushSubscription, n domain.Notification) ([]byte, error) {
	lang := i18n.Negotiate(sub.Lang)
	return json.Marshal(message{
		Title: i18n.T(lang, n.Key+".title", n.Args...),
		Body:  i18n.T(lang, n.Key+".body", n.Args...),
		URL:   n.URL,
		Tag:   n.Tag,
	})
}

func (p *Pusher) Send(ctx context.Context, sub domain.PushSubscription, n domain.Notification) error {
	body, err := render(sub, n)
	if err != nil {
		return err
	}
	resp, err := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		HTTPClient:      p.client,
		Subscriber:      p.subject,
		VAPIDPublicKey:  p.publicKey,
		VAPIDPrivateKey: p.privateKey,
		TTL:             ttl,
		Urgency:         webpush.UrgencyNormal,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return fmt.Errorf("%w: push service dropped the subscription", domain.ErrNotFound)
	case resp.StatusCode >= 300:
		return fmt.Errorf("push service answered %d", resp.StatusCode)
	}
	return nil
}
