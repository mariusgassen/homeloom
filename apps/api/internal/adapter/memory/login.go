package memory

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// LoginProvider is a stand-in for authentik in tests. It accepts the code "code-<subject>"
// for any login it started, fails with an availability error for "code-down", and rejects
// everything else.
type LoginProvider struct {
	mu     sync.Mutex
	issued map[string]string // nonce -> verifier
}

func NewLoginProvider() *LoginProvider { return &LoginProvider{issued: map[string]string{}} }

func (p *LoginProvider) AuthCodeURL(_ context.Context, state, nonce, verifier string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.issued[nonce] = verifier
	return "https://idp.test/authorize?state=" + url.QueryEscape(state), nil
}

func (p *LoginProvider) Exchange(_ context.Context, code, verifier, nonce string) (domain.Identity, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sub, ok := strings.CutPrefix(code, "code-")
	switch {
	case sub == "down":
		return domain.Identity{}, "", errors.New("authentik unreachable")
	case !ok || p.issued[nonce] != verifier:
		return domain.Identity{}, "", fmt.Errorf("%w: bad code", domain.ErrUnauthenticated)
	}
	return domain.Identity{Subject: sub, Name: sub}, "idtoken-" + sub, nil
}

func (p *LoginProvider) LogoutURL(_ context.Context, idToken string) (string, error) {
	return "https://idp.test/end-session/?id_token_hint=" + url.QueryEscape(idToken), nil
}
