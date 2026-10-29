// Package secretbox encrypts secrets before another adapter stores them. HomeConnections and
// WatchConnections wrap their repositories so the database only ever sees sealed Home Assistant
// and Plex tokens.
package secretbox

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Box seals with AES-256-GCM.
type Box struct {
	aead cipher.AEAD
}

// New takes the key as base64 of 32 random bytes (openssl rand -base64 32).
func New(key string) (*Box, error) {
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("HOMELOOM_SECRET_KEY must be 32 random bytes, base64-encoded (openssl rand -base64 32)")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal returns the nonce and ciphertext, base64-encoded. ad binds it to where it is stored.
func (b *Box) Seal(plain string, ad []byte) string {
	nonce := make([]byte, b.aead.NonceSize())
	_, _ = rand.Read(nonce) // crypto/rand.Read never fails
	return base64.RawStdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, []byte(plain), ad))
}

func (b *Box) Open(sealed string, ad []byte) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", errors.New("secretbox: malformed")
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], ad)
	if err != nil {
		return "", errors.New("secretbox: cannot open (wrong HOMELOOM_SECRET_KEY?)")
	}
	return string(plain), nil
}

type HomeConnections struct {
	next port.HomeConnectionRepository
	box  *Box
}

var _ port.HomeConnectionRepository = (*HomeConnections)(nil)

func NewHomeConnections(next port.HomeConnectionRepository, box *Box) *HomeConnections {
	return &HomeConnections{next: next, box: box}
}

func (h *HomeConnections) SaveHomeConnection(ctx context.Context, c domain.HomeConnection) error {
	c.RefreshToken = h.box.Seal(c.RefreshToken, c.UserID[:])
	return h.next.SaveHomeConnection(ctx, c)
}

// HomeConnection treats a token it can't open (say, after the key changed) as no connection,
// so the person is asked to connect again rather than locked out.
func (h *HomeConnections) HomeConnection(ctx context.Context, userID uuid.UUID) (domain.HomeConnection, error) {
	c, err := h.next.HomeConnection(ctx, userID)
	if err != nil {
		return c, err
	}
	if c.RefreshToken, err = h.box.Open(c.RefreshToken, userID[:]); err != nil {
		return domain.HomeConnection{}, fmt.Errorf("%w: %v", domain.ErrNotFound, err) //nolint:errorlint // the cause is only for the log
	}
	return c, nil
}

func (h *HomeConnections) DeleteHomeConnection(ctx context.Context, userID uuid.UUID) error {
	return h.next.DeleteHomeConnection(ctx, userID)
}

type WatchConnections struct {
	next port.WatchConnectionRepository
	box  *Box
}

var _ port.WatchConnectionRepository = (*WatchConnections)(nil)

func NewWatchConnections(next port.WatchConnectionRepository, box *Box) *WatchConnections {
	return &WatchConnections{next: next, box: box}
}

// watchAD keeps a sealed Plex token from opening as a Home Assistant one, and the other way round.
func watchAD(userID uuid.UUID) []byte { return append([]byte("watch:"), userID[:]...) }

func (w *WatchConnections) SaveWatchConnection(ctx context.Context, c domain.WatchConnection) error {
	c.Token = w.box.Seal(c.Token, watchAD(c.UserID))
	return w.next.SaveWatchConnection(ctx, c)
}

// WatchConnection treats a token it can't open as no connection, like HomeConnection.
func (w *WatchConnections) WatchConnection(ctx context.Context, userID uuid.UUID) (domain.WatchConnection, error) {
	c, err := w.next.WatchConnection(ctx, userID)
	if err != nil {
		return c, err
	}
	if c.Token, err = w.box.Open(c.Token, watchAD(userID)); err != nil {
		return domain.WatchConnection{}, fmt.Errorf("%w: %v", domain.ErrNotFound, err) //nolint:errorlint // the cause is only for the log
	}
	return c, nil
}

func (w *WatchConnections) DeleteWatchConnection(ctx context.Context, userID uuid.UUID) error {
	return w.next.DeleteWatchConnection(ctx, userID)
}
