package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	gooidc "github.com/coreos/go-oidc/v3/oidc"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// Verifier accepts JWT access tokens for any of the configured client IDs (one per Homeloom
// client: PWA, iOS, MCP, ...). The first provider is the main issuer; further providers, one per
// extra authentik provider, are used for tokens whose issuer names them.
type Verifier struct {
	clientIDs []string
	issuers   []*issuerVerifier
}

type issuerVerifier struct {
	provider *Provider

	mu       sync.Mutex
	verifier *gooidc.IDTokenVerifier
}

func NewVerifier(provider *Provider, clientIDs []string, extra ...*Provider) *Verifier {
	v := &Verifier{clientIDs: clientIDs, issuers: []*issuerVerifier{{provider: provider}}}
	for _, p := range extra {
		v.issuers = append(v.issuers, &issuerVerifier{provider: p})
	}
	return v
}

func (v *Verifier) Verify(ctx context.Context, rawToken string) (domain.Identity, error) {
	verifier, err := v.pick(rawToken).get(ctx)
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

// pick chooses the issuer a token names. The claim is read before the signature is checked, but
// only to choose among configured issuers; the chosen one then verifies signature and issuer, so
// a forged claim gains nothing. An unknown or unreadable issuer falls to the main one, which
// rejects it.
func (v *Verifier) pick(rawToken string) *issuerVerifier {
	if len(v.issuers) > 1 {
		if iss := unverifiedIssuer(rawToken); iss != "" {
			for _, i := range v.issuers {
				if i.provider.issuer == iss {
					return i
				}
			}
		}
	}
	return v.issuers[0]
}

func unverifiedIssuer(rawToken string) string {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var c struct {
		Issuer string `json:"iss"`
	}
	if json.Unmarshal(payload, &c) != nil {
		return ""
	}
	return c.Issuer
}

func (i *issuerVerifier) get(ctx context.Context) (*gooidc.IDTokenVerifier, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.verifier != nil {
		return i.verifier, nil
	}
	provider, err := i.provider.get(ctx)
	if err != nil {
		return nil, err
	}
	// Audience is checked against the list in Verify.
	i.verifier = provider.Verifier(&gooidc.Config{SkipClientIDCheck: true})
	return i.verifier, nil
}
