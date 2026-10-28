package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Login runs the browser sign-in. The client secret and all tokens stay on the server;
// the browser only ever holds the state and, afterwards, the session id.
type Login struct {
	provider port.LoginProvider
	sessions port.SessionStore
}

func NewLogin(provider port.LoginProvider, sessions port.SessionStore) *Login {
	return &Login{provider: provider, sessions: sessions}
}

func (s *Login) Begin(ctx context.Context) (string, string, error) {
	state, nonce, verifier := randomID(), randomID(), randomID()
	url, err := s.provider.AuthCodeURL(ctx, state, nonce, verifier)
	if err != nil {
		return "", "", err
	}
	if err := s.sessions.SaveLogin(ctx, state, domain.PendingLogin{Nonce: nonce, Verifier: verifier}); err != nil {
		return "", "", err
	}
	return state, url, nil
}

func (s *Login) Complete(ctx context.Context, state, browserState, code string) (string, error) {
	// The state must come back to the browser that started the login (CSRF on the callback).
	if state == "" || code == "" || subtle.ConstantTimeCompare([]byte(state), []byte(browserState)) != 1 {
		return "", fmt.Errorf("%w: state mismatch", domain.ErrUnauthenticated)
	}
	pending, err := s.sessions.TakeLogin(ctx, state)
	if errors.Is(err, domain.ErrNotFound) {
		return "", fmt.Errorf("%w: unknown or expired login", domain.ErrUnauthenticated)
	}
	if err != nil {
		return "", err
	}
	id, idToken, err := s.provider.Exchange(ctx, code, pending.Verifier, pending.Nonce)
	if err != nil {
		return "", err
	}
	if id.Subject == "" {
		return "", fmt.Errorf("%w: id token has no subject", domain.ErrUnauthenticated)
	}
	sessionID := randomID()
	if err := s.sessions.Save(ctx, sessionID, domain.Session{Identity: id, IDToken: idToken}); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *Login) Logout(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		return "", nil
	}
	sess, err := s.sessions.Get(ctx, sessionID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if err := s.sessions.Delete(ctx, sessionID); err != nil {
		return "", err
	}
	return s.provider.LogoutURL(ctx, sess.IDToken)
}

// randomID returns 256 random bits, base64url-encoded (43 characters, also a valid PKCE verifier).
func randomID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails
	return base64.RawURLEncoding.EncodeToString(b)
}
