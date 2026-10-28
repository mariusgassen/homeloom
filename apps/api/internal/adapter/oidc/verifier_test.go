package oidc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/oidc"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// issuer is a minimal OIDC provider: discovery document, JWKS and a token minter.
type issuer struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	signer jose.Signer
}

func newIssuer(t *testing.T) *issuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "k1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	is := &issuer{key: key, signer: signer}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                is.srv.URL,
			"jwks_uri":                              is.srv.URL + "/jwks",
			"authorization_endpoint":                is.srv.URL + "/authorize",
			"token_endpoint":                        is.srv.URL + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	is.srv = httptest.NewServer(mux)
	t.Cleanup(is.srv.Close)
	return is
}

func (is *issuer) token(t *testing.T, aud string, exp time.Time) string {
	t.Helper()
	raw, err := jwt.Signed(is.signer).Claims(jwt.Claims{
		Issuer: is.srv.URL, Subject: "user-1", Audience: jwt.Audience{aud}, Expiry: jwt.NewNumericDate(exp),
	}).Claims(map[string]any{"name": "Alice", "email": "alice@example.com"}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestVerifier(t *testing.T) {
	is := newIssuer(t)
	v := oidc.NewVerifier(is.srv.URL, []string{"homeloom-pwa", "homeloom-ios"})
	ctx := context.Background()

	id, err := v.Verify(ctx, is.token(t, "homeloom-ios", time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "user-1" || id.Name != "Alice" || id.Email != "alice@example.com" {
		t.Fatalf("got %+v", id)
	}

	for name, tok := range map[string]string{
		"other client": is.token(t, "someone-else", time.Now().Add(time.Hour)),
		"expired":      is.token(t, "homeloom-pwa", time.Now().Add(-time.Hour)),
		"garbage":      "not-a-jwt",
	} {
		if _, err := v.Verify(ctx, tok); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("%s: want ErrUnauthenticated, got %v", name, err)
		}
	}
}

func TestVerifierProviderDownIsNotAnAuthFailure(t *testing.T) {
	v := oidc.NewVerifier("http://127.0.0.1:1", []string{"homeloom-pwa"})
	_, err := v.Verify(context.Background(), "whatever")
	if err == nil || errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("want an availability error, got %v", err)
	}
}
