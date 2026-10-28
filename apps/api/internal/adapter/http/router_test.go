package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpadapter "github.com/mariusgassen/homeloom/apps/api/internal/adapter/http"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

// fakeVerifier accepts "token-<subject>" and fails with a non-auth error for "token-down".
type fakeVerifier struct{}

func (fakeVerifier) Verify(_ context.Context, raw string) (domain.Identity, error) {
	sub, ok := strings.CutPrefix(raw, "token-")
	switch {
	case !ok:
		return domain.Identity{}, domain.ErrUnauthenticated
	case sub == "down":
		return domain.Identity{}, errors.New("authentik unreachable")
	}
	return domain.Identity{Subject: sub, Name: sub}, nil
}

type failingCheck struct{}

func (failingCheck) Name() string                { return "redis" }
func (failingCheck) Check(context.Context) error { return errors.New("down") }

func newServer(t *testing.T, checks ...port.HealthCheck) http.Handler {
	t.Helper()
	store := memory.NewStore()
	h, err := httpadapter.NewRouter(httpadapter.Deps{
		Identity:     service.NewIdentity(fakeVerifier{}, store),
		Households:   service.NewHousehold(store),
		HealthChecks: checks,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(t *testing.T, h http.Handler, method, path, token, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestHealthNeedsNoToken(t *testing.T) {
	if code, _ := do(t, newServer(t), "GET", "/healthz", "", ""); code != 200 {
		t.Fatalf("got %d", code)
	}
}

func TestReadinessReportsFailingDependency(t *testing.T) {
	code, body := do(t, newServer(t, failingCheck{}), "GET", "/readyz", "", "")
	if code != 503 || body["status"] != "degraded" {
		t.Fatalf("got %d %v", code, body)
	}
}

func TestAuth(t *testing.T) {
	h := newServer(t)
	cases := []struct {
		name, token string
		want        int
	}{
		{"no token", "", 401},
		{"bad token", "garbage", 401},
		{"provider down", "token-down", 503},
		{"valid", "token-alice", 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code, body := do(t, h, "GET", "/v1/me", c.token, ""); code != c.want {
				t.Fatalf("want %d, got %d %v", c.want, code, body)
			}
		})
	}
}

func TestHouseholdFlow(t *testing.T) {
	h := newServer(t)

	if code, _ := do(t, h, "POST", "/v1/households", "token-alice", `{}`); code != 400 {
		t.Fatalf("missing name: want 400 from spec validation, got %d", code)
	}
	if code, _ := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"   "}`); code != 400 {
		t.Fatalf("blank name: want 400 from core, got %d", code)
	}

	code, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	if code != 201 || created["role"] != "owner" {
		t.Fatalf("create: %d %v", code, created)
	}
	id := created["id"].(string)

	if code, body := do(t, h, "GET", "/v1/households/"+id, "token-alice", ""); code != 200 || len(body["members"].([]any)) != 1 {
		t.Fatalf("owner get: %d %v", code, body)
	}
	if code, _ := do(t, h, "GET", "/v1/households/"+id, "token-eve", ""); code != 404 {
		t.Fatalf("non-member get: want 404, got %d", code)
	}
	if code, _ := do(t, h, "GET", "/v1/households/not-a-uuid", "token-alice", ""); code != 400 {
		t.Fatalf("bad id: want 400, got %d", code)
	}
}
