// Package oidc verifies access tokens issued by authentik (or any OIDC provider).
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
// (one per Homeloom client: PWA, iOS, MCP, ...). Discovery runs on first use rather than at
// startup, so the API still boots, and serves health checks, while authentik is down.
type Verifier struct {
	issuer    string
	clientIDs []string

	mu       sync.Mutex
	verifier *gooidc.IDTokenVerifier
}

func NewVerifier(issuer string, clientIDs []string) *Verifier {
	return &Verifier{issuer: issuer, clientIDs: clientIDs}
}

type claims struct {
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	Picture           string `json:"picture"`
}

func (v *Verifier) Verify(ctx context.Context, rawToken string) (domain.Identity, error) {
	verifier, err := v.get(ctx)
	if err != nil {
		return domain.Identity{}, err
	}
	tok, err := verifier.Verify(ctx, rawToken)
	if err != nil {
		return domain.Identity{}, fmt.Errorf("%w: %v", domain.ErrUnauthenticated, err)
	}
	if !slices.ContainsFunc(tok.Audience, func(aud string) bool { return slices.Contains(v.clientIDs, aud) }) {
		return domain.Identity{}, fmt.Errorf("%w: unexpected audience", domain.ErrUnauthenticated)
	}
	var c claims
	if err := tok.Claims(&c); err != nil {
		return domain.Identity{}, fmt.Errorf("%w: %v", domain.ErrUnauthenticated, err)
	}
	name := c.Name
	if name == "" {
		name = c.PreferredUsername
	}
	return domain.Identity{Subject: tok.Subject, Name: name, Email: c.Email, AvatarURL: c.Picture}, nil
}

func (v *Verifier) get(ctx context.Context) (*gooidc.IDTokenVerifier, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.verifier != nil {
		return v.verifier, nil
	}
	// The provider outlives this request, so it must not inherit its cancellation.
	provider, err := gooidc.NewProvider(context.WithoutCancel(ctx), v.issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery for %s: %w", v.issuer, err)
	}
	// Audience is checked against the list in Verify.
	v.verifier = provider.Verifier(&gooidc.Config{SkipClientIDCheck: true})
	return v.verifier, nil
}
