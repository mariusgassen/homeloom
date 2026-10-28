package oidc

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// exchangeTimeout bounds the token request to the provider.
const exchangeTimeout = 10 * time.Second

// Login runs the authorization code flow (with PKCE and nonce) for the confidential web client.
type Login struct {
	provider      *Provider
	clientID      string
	clientSecret  string
	redirectURL   string
	postLogoutURL string
}

func NewLogin(provider *Provider, clientID, clientSecret, redirectURL, postLogoutURL string) *Login {
	return &Login{provider: provider, clientID: clientID, clientSecret: clientSecret, redirectURL: redirectURL, postLogoutURL: postLogoutURL}
}

func (l *Login) config(p *gooidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     l.clientID,
		ClientSecret: l.clientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  l.redirectURL,
		Scopes:       []string{gooidc.ScopeOpenID, "profile", "email"},
	}
}

func (l *Login) AuthCodeURL(ctx context.Context, state, nonce, verifier string) (string, error) {
	p, err := l.provider.get(ctx)
	if err != nil {
		return "", err
	}
	return l.config(p).AuthCodeURL(state, gooidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

func (l *Login) Exchange(ctx context.Context, code, verifier, nonce string) (domain.Identity, string, error) {
	p, err := l.provider.get(ctx)
	if err != nil {
		return domain.Identity{}, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, exchangeTimeout)
	defer cancel()
	tok, err := l.config(p).Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && re.Response != nil && re.Response.StatusCode < 500 {
			return domain.Identity{}, "", fmt.Errorf("%w: %w", domain.ErrUnauthenticated, err)
		}
		return domain.Identity{}, "", fmt.Errorf("oidc token exchange: %w", err)
	}
	rawID, _ := tok.Extra("id_token").(string)
	if rawID == "" {
		return domain.Identity{}, "", fmt.Errorf("%w: no id_token in token response", domain.ErrUnauthenticated)
	}
	idTok, err := p.Verifier(&gooidc.Config{ClientID: l.clientID}).Verify(ctx, rawID)
	if err != nil {
		return domain.Identity{}, "", fmt.Errorf("%w: %w", domain.ErrUnauthenticated, err)
	}
	if idTok.Nonce != nonce {
		return domain.Identity{}, "", fmt.Errorf("%w: nonce mismatch", domain.ErrUnauthenticated)
	}
	var c claims
	if err := idTok.Claims(&c); err != nil {
		return domain.Identity{}, "", fmt.Errorf("%w: %w", domain.ErrUnauthenticated, err)
	}
	return c.identity(idTok.Subject), rawID, nil
}

func (l *Login) LogoutURL(ctx context.Context, idToken string) (string, error) {
	p, err := l.provider.get(ctx)
	if err != nil {
		return "", err
	}
	var meta struct {
		EndSession string `json:"end_session_endpoint"`
	}
	if err := p.Claims(&meta); err != nil || meta.EndSession == "" {
		return "", nil //nolint:nilerr // a provider without an end-session endpoint is not an error
	}
	u, err := url.Parse(meta.EndSession)
	if err != nil {
		return "", fmt.Errorf("oidc end_session_endpoint: %w", err)
	}
	q := u.Query()
	q.Set("id_token_hint", idToken)
	q.Set("post_logout_redirect_uri", l.postLogoutURL)
	q.Set("client_id", l.clientID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
