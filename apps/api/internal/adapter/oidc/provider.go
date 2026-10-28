// Package oidc talks to authentik (or any OIDC provider): it verifies access tokens and runs
// the browser login.
package oidc

import (
	"context"
	"fmt"
	"sync"

	gooidc "github.com/coreos/go-oidc/v3/oidc"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// Provider discovers the issuer on first use rather than at startup, so the API still boots,
// and serves health checks, while authentik is down.
type Provider struct {
	issuer string

	mu       sync.Mutex
	provider *gooidc.Provider
}

func NewProvider(issuer string) *Provider {
	return &Provider{issuer: issuer}
}

func (p *Provider) get(ctx context.Context) (*gooidc.Provider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider != nil {
		return p.provider, nil
	}
	// The provider outlives this request, so it must not inherit its cancellation.
	provider, err := gooidc.NewProvider(context.WithoutCancel(ctx), p.issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery for %s: %w", p.issuer, err)
	}
	p.provider = provider
	return provider, nil
}

type claims struct {
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	Picture           string `json:"picture"`
}

func (c claims) identity(subject string) domain.Identity {
	name := c.Name
	if name == "" {
		name = c.PreferredUsername
	}
	return domain.Identity{Subject: subject, Name: name, Email: c.Email, AvatarURL: c.Picture}
}
