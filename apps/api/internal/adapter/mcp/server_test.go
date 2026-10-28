package mcp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	mcpadapter "github.com/mariusgassen/homeloom/apps/api/internal/adapter/mcp"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
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

type fixture struct {
	url        string
	identity   *service.Identity
	households *service.Household
	agents     *service.AgentAccess
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store, sessions := memory.NewStore(), memory.NewSessions()
	push := service.NewPush(store, &memory.Pusher{})
	entries := service.NewEntry(store, store, store, push, time.UTC)
	shopping := service.NewShopping(store, store)
	cook := service.NewCook(store, nil, nil, shopping)
	smart := memory.NewSmartHome([]domain.Scene{{ID: "scene.dinner", Name: "Dinner"}}, nil)
	home := service.NewHome(store, store, sessions, smart)
	f := &fixture{
		identity:   service.NewIdentity(fakeVerifier{}, sessions, store),
		households: service.NewHousehold(store),
		agents:     service.NewAgentAccess(store, sessions, store),
	}
	srv := httptest.NewServer(mcpadapter.NewHandler(mcpadapter.Deps{
		Version:    "1.2.3",
		Identity:   f.identity,
		Agents:     f.agents,
		Households: f.households,
		Tools:      service.NewAssistant(store, store, nil, entries, cook, shopping, home, time.UTC),
	}))
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f
}

func (f *fixture) user(t *testing.T, token string) domain.User {
	t.Helper()
	u, err := f.identity.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func connect(t *testing.T, url, token string) (*sdk.ClientSession, error) {
	t.Helper()
	c := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	return c.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint: url, HTTPClient: &http.Client{Transport: bearer{token}}, MaxRetries: -1, DisableStandaloneSSE: true,
	}, nil)
}

func call(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var out strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			out.WriteString(tc.Text)
		}
	}
	return out.String(), res.IsError
}

func TestToolsActAsTheCaller(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	alice := f.user(t, "token-alice")
	m, err := f.households.Create(ctx, alice, "Home")
	if err != nil {
		t.Fatal(err)
	}

	cs, err := connect(t, f.url, "token-alice")
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if cs.InitializeResult().Instructions == "" {
		t.Fatal("no instructions")
	}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) == 0 {
		t.Fatalf("tools: %v, %v", tools, err)
	}
	if out, isErr := call(t, cs, "add_reminder", map[string]any{"text": "Call grandma", "visibility": "private"}); isErr {
		t.Fatal(out)
	}
	if out, _ := call(t, cs, "list_entries", map[string]any{"kind": "reminder"}); !strings.Contains(out, "Call grandma") {
		t.Fatalf("alice's reminders: %s", out)
	}
	if out, isErr := call(t, cs, "list_entries", map[string]any{"kind": "chore"}); !isErr {
		t.Fatalf("a wrong kind is the model's to fix: %s", out)
	}

	// The voice agent's token is bound to Alice in this household.
	token, err := f.agents.Mint(ctx, alice, m.Household.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := connect(t, f.url, token)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	if out, _ := call(t, agent, "list_entries", map[string]any{"kind": "reminder"}); !strings.Contains(out, "Call grandma") {
		t.Fatalf("the agent's view: %s", out)
	}

	// So is a personal token she made for an app, until she deletes it.
	tok, secret, err := f.agents.CreateToken(ctx, alice, m.Household.ID, "Claude")
	if err != nil {
		t.Fatal(err)
	}
	app, err := connect(t, f.url, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if out, _ := call(t, app, "list_entries", map[string]any{"kind": "reminder"}); !strings.Contains(out, "Call grandma") {
		t.Fatalf("the app's view: %s", out)
	}
	if err := f.agents.DeleteToken(ctx, alice, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CallTool(ctx, &sdk.CallToolParams{Name: "list_entries", Arguments: map[string]any{"kind": "reminder"}}); err == nil {
		t.Fatal("a deleted token still works")
	}

	// Eve isn't a member: asking for the household gets her nothing.
	f.user(t, "token-eve")
	eve, err := connect(t, f.url+"?household="+m.Household.ID.String(), "token-eve")
	if err != nil {
		t.Fatal(err)
	}
	defer eve.Close()
	if out, isErr := call(t, eve, "list_entries", map[string]any{"kind": "reminder"}); !isErr || out != "not found" {
		t.Fatalf("eve: %s", out)
	}
}

func TestAuthentication(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	alice := f.user(t, "token-alice")
	for _, name := range []string{"Home", "Cabin"} {
		if _, err := f.households.Create(ctx, alice, name); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		token, query string
		status       int
	}{
		{"", "", http.StatusUnauthorized},
		{"nonsense", "", http.StatusUnauthorized},
		{domain.AgentTokenPrefix + "made-up", "", http.StatusUnauthorized},
		{domain.MCPTokenPrefix + "made-up", "", http.StatusUnauthorized},
		{"token-down", "", http.StatusServiceUnavailable},
		// With two households she has to pick one.
		{"token-alice", "", http.StatusBadRequest},
		{"token-alice", "?household=cabin", http.StatusBadRequest},
	} {
		req, _ := http.NewRequest("POST", f.url+tc.query, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Errorf("%q%s: got %d, want %d", tc.token, tc.query, resp.StatusCode, tc.status)
		}
		if tc.status == http.StatusUnauthorized && resp.Header.Get("WWW-Authenticate") == "" {
			t.Errorf("%q: no WWW-Authenticate", tc.token)
		}
	}
}
