package oidc

import (
	"context"
	"fmt"
	"slices"
	"sync"

	gooidc "github.com/coreos/go-oidc/v3/oidc"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// Verifier accepts JWT access tokens from one issuer, for any of the configured client IDs
// (one per Homeloom client: PWA, iOS, MCP, ...).
type Verifier struct {
	provider  *Provider
	clientIDs []string

	mu       sync.Mutex
	verifier *gooidc.IDTokenVerifier
}

func NewVerifier(provider *Provider, clientIDs []string) *Verifier {
	return &Verifier{provider: provider, clientIDs: clientIDs}
}

func (v *Verifier) Verify(ctx context.Context, rawToken string) (domain.Identity, error) {
	verifier, err := v.get(ctx)
	if err != nil {
		return domain.Identity{}, err
	}
	tok, err := verifier.Verify(ctx, rawToken)
	if err != nil {
		return domain.Identity{}, fmt.Errorf("%w: %w", domain.ErrUnauthenticated, err)
	}
	if !slices.ContainsFunc(tok.Audience, func(aud string) bool { return slices.Contains(v.clientIDs, aud) }) {
		return domain.Identity{}, fmt.Errorf("%w: unexpected audience", domain.ErrUnauthenticated)
	}
	var c claims
	if err := tok.Claims(&c); err != nil {
		return domain.Identity{}, fmt.Errorf("%w: %w", domain.ErrUnauthenticated, err)
	}
	return c.identity(tok.Subject), nil
}

func (v *Verifier) get(ctx context.Context) (*gooidc.IDTokenVerifier, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.verifier != nil {
		return v.verifier, nil
	}
	provider, err := v.provider.get(ctx)
	if err != nil {
		return nil, err
	}
	// Audience is checked against the list in Verify.
	v.verifier = provider.Verifier(&gooidc.Config{SkipClientIDCheck: true})
	return v.verifier, nil
}
