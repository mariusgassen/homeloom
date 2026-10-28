package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	return newServerWith(t, memory.NewRecipeBook(curry), checks...)
}

var curry = domain.Recipe{
	RecipeSummary: domain.RecipeSummary{Slug: "green-curry", Name: "Green curry", Tags: []string{"Thai"}},
	Ingredients:   []domain.Ingredient{{ID: "i1", Text: "Coconut milk"}, {ID: "i2", Text: "Rice"}},
	Steps:         []string{"Cook."},
}

// newServerWith serves the API with the given Mealie stand-in; nil means Mealie isn't set up.
func newServerWith(t *testing.T, book *memory.RecipeBook, checks ...port.HealthCheck) http.Handler {
	t.Helper()
	store, sessions := memory.NewStore(), memory.NewSessions()
	var recipes port.RecipeBook
	if book != nil {
		recipes = book
	}
	smart := memory.NewSmartHome([]domain.Scene{{ID: "scene.dinner", Name: "Dinner"}}, []domain.Light{{ID: "light.kitchen", Name: "Kitchen"}})
	h, err := httpadapter.NewRouter(httpadapter.Deps{
		Version:       "1.2.3",
		Identity:      service.NewIdentity(fakeVerifier{}, sessions, store),
		Login:         service.NewLogin(memory.NewLoginProvider(), sessions),
		Households:    service.NewHousehold(store),
		Questions:     service.NewQuestion(store, store),
		Cook:          service.NewCook(store, recipes, memory.Chef{}),
		Home:          service.NewHome(store, smart),
		HealthChecks:  checks,
		SecureCookies: true,
		SessionTTL:    time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(t *testing.T, h http.Handler, method, path, token, body string, headers ...string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
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

func TestHealthNeedsNoTokenAndReportsVersion(t *testing.T) {
	if code, body := do(t, newServer(t), "GET", "/healthz", "", ""); code != 200 || body["version"] != "1.2.3" {
		t.Fatalf("got %d %v", code, body)
	}
}

func TestErrorsFollowAcceptLanguage(t *testing.T) {
	h := newServer(t)
	_, en := do(t, h, "GET", "/v1/me", "", "")
	_, de := do(t, h, "GET", "/v1/me", "", "", "Accept-Language", "de-DE,de;q=0.9")
	if en["code"] != "unauthenticated" || de["code"] != "unauthenticated" {
		t.Fatalf("codes must not be translated: %v %v", en, de)
	}
	if en["message"] != "Please sign in again." || de["message"] != "Bitte melde dich erneut an." {
		t.Fatalf("got en=%q de=%q", en["message"], de["message"])
	}

	_, bad := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"   "}`, "Accept-Language", "de")
	if bad["message"] != "Gib dem Haushalt einen Namen mit 1 bis 100 Zeichen." {
		t.Fatalf("got %v", bad)
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

func TestTogetherFlow(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	id := created["id"].(string)

	if code, _ := do(t, h, "POST", "/v1/households/"+id+"/invites", "token-eve", ""); code != 404 {
		t.Fatalf("non-member invite: want 404, got %d", code)
	}
	code, inv := do(t, h, "POST", "/v1/households/"+id+"/invites", "token-alice", "")
	if code != 201 || inv["code"] == "" || inv["expiresAt"] == nil {
		t.Fatalf("invite: %d %v", code, inv)
	}
	if code, joined := do(t, h, "POST", "/v1/invites/"+inv["code"].(string), "token-bob", ""); code != 200 || joined["id"] != id || joined["role"] != "member" {
		t.Fatalf("join: %d %v", code, joined)
	}
	if code, body := do(t, h, "POST", "/v1/invites/"+inv["code"].(string), "token-eve", "", "Accept-Language", "de"); code != 404 || body["code"] != "invite_invalid" || !strings.Contains(body["message"].(string), "gilt nicht mehr") {
		t.Fatalf("reused invite: %d %v", code, body)
	}

	if code, body := do(t, h, "POST", "/v1/households/"+id+"/questions", "token-alice", `{"text":"  "}`); code != 400 || body["code"] != "invalid_text" {
		t.Fatalf("blank question: %d %v", code, body)
	}
	code, q := do(t, h, "POST", "/v1/households/"+id+"/questions", "token-alice", `{"text":"Best moment today?"}`)
	if code != 201 || q["revealed"] != false {
		t.Fatalf("ask: %d %v", code, q)
	}
	answer := "/v1/questions/" + q["id"].(string) + "/answer"
	if code, _ := do(t, h, "PUT", answer, "token-eve", `{"text":"hi"}`); code != 404 {
		t.Fatalf("non-member answer: want 404, got %d", code)
	}
	if code, _ := do(t, h, "PUT", answer, "token-alice", `{"text":"Breakfast"}`); code != 200 {
		t.Fatalf("alice answers: %d", code)
	}

	// Bob sees that Alice answered, but not what.
	req := httptest.NewRequest("GET", "/v1/households/"+id+"/questions", nil)
	req.Header.Set("Authorization", "Bearer token-bob")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list: %v %s", err, rec.Body)
	}
	sealed := list[0]["answers"].([]any)[0].(map[string]any)
	if _, hasText := sealed["text"]; hasText {
		t.Fatalf("bob must not see alice's text: %v", sealed)
	}

	code, q = do(t, h, "PUT", answer, "token-bob", `{"text":"The walk"}`)
	if code != 200 || q["revealed"] != true || len(q["answers"].([]any)) != 2 {
		t.Fatalf("bob answers, question opens: %d %v", code, q)
	}
	if code, body := do(t, h, "PUT", answer, "token-alice", `{"text":"changed"}`); code != 409 || body["code"] != "already_revealed" {
		t.Fatalf("change after reveal: %d %v", code, body)
	}
}

type response struct {
	StatusCode int
	Header     http.Header
	cookies    []*http.Cookie
	body       []byte
}

func TestCookFlow(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	base := "/v1/households/" + created["id"].(string)

	code, s := do(t, h, "POST", base+"/cook/suggestions", "token-alice", `{"mood":"curry tonight"}`, "Accept-Language", "de")
	picks, _ := s["suggestions"].([]any)
	if code != 200 || s["chef"] != true || len(picks) != 1 {
		t.Fatalf("suggest: %d %v", code, s)
	}
	if first := picks[0].(map[string]any); first["reason"] != "de: fits curry" || first["recipe"].(map[string]any)["slug"] != "green-curry" {
		t.Fatalf("suggestion: %v", first)
	}
	if code, body := do(t, h, "POST", base+"/cook/suggestions", "token-eve", `{"mood":"curry"}`); code != 404 {
		t.Fatalf("non-member: %d %v", code, body)
	}

	code, r := do(t, h, "GET", base+"/recipes/green-curry", "token-alice", "")
	if code != 200 || len(r["ingredients"].([]any)) != 2 || r["steps"].([]any)[0] != "Cook." {
		t.Fatalf("recipe: %d %v", code, r)
	}
	if code, _ := do(t, h, "GET", base+"/recipes/no..pe", "token-alice", ""); code != 400 {
		t.Fatalf("bad slug: want 400, got %d", code)
	}
	if code, _ := do(t, h, "GET", base+"/recipes/nope", "token-alice", ""); code != 404 {
		t.Fatalf("unknown recipe: want 404, got %d", code)
	}

	code, added := do(t, h, "POST", base+"/shopping-list", "token-alice", `{"recipe":"green-curry","ingredientIds":["i2"]}`)
	if code != 200 || added["added"] != 1.0 || added["list"] != "Groceries" {
		t.Fatalf("add: %d %v", code, added)
	}
	if code, body := do(t, h, "POST", base+"/shopping-list", "token-alice", `{"recipe":"green-curry","ingredientIds":["x"]}`); code != 400 || body["code"] != "invalid_ingredients" {
		t.Fatalf("foreign ingredient: %d %v", code, body)
	}
}

func TestCookingWithoutMealie(t *testing.T) {
	h := newServerWith(t, nil)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	code, body := do(t, h, "POST", "/v1/households/"+created["id"].(string)+"/cook/suggestions", "token-alice", `{"mood":"cozy"}`, "Accept-Language", "de")
	if code != 503 || body["code"] != "cooking_unavailable" || !strings.Contains(body["message"].(string), "Mealie") {
		t.Fatalf("no Mealie: %d %v", code, body)
	}
}

func browser(t *testing.T, h http.Handler, method, path string, cookies ...*http.Cookie) response {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	return response{StatusCode: res.StatusCode, Header: res.Header, cookies: res.Cookies(), body: rec.Body.Bytes()}
}

func cookie(t *testing.T, res response, name string) *http.Cookie {
	t.Helper()
	for _, c := range res.cookies {
		if c.Name == name {
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
				t.Errorf("cookie %s is not HttpOnly, Secure, SameSite=Lax: %+v", name, c)
			}
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", name, res.Header["Set-Cookie"])
	return nil
}

func TestBrowserLogin(t *testing.T) {
	h := newServer(t)

	res := browser(t, h, "GET", "/auth/login")
	if res.StatusCode != http.StatusFound || !strings.HasPrefix(res.Header.Get("Location"), "https://idp.test/authorize?") {
		t.Fatalf("login: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	loginCookie := cookie(t, res, "homeloom_login")
	if loginCookie.Path != "/auth/callback" {
		t.Errorf("login cookie path %q", loginCookie.Path)
	}

	res = browser(t, h, "GET", "/auth/callback?code=code-alice&state="+loginCookie.Value, loginCookie)
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/" {
		t.Fatalf("callback: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	session := cookie(t, res, "homeloom_session")
	if session.Path != "/" || session.MaxAge != 3600 {
		t.Errorf("session cookie %+v", session)
	}

	res = browser(t, h, "GET", "/v1/me", session)
	var me map[string]any
	_ = json.Unmarshal(res.body, &me)
	if res.StatusCode != http.StatusOK || me["name"] != "alice" {
		t.Fatalf("me with cookie: %d %v", res.StatusCode, me)
	}

	res = browser(t, h, "POST", "/auth/logout", session)
	var out map[string]any
	_ = json.Unmarshal(res.body, &out)
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(out["redirectUrl"].(string), "https://idp.test/end-session/") {
		t.Fatalf("logout: %d %v", res.StatusCode, out)
	}
	if c := cookie(t, res, "homeloom_session"); c.MaxAge >= 0 {
		t.Errorf("logout must expire the cookie: %+v", c)
	}
	if res := browser(t, h, "GET", "/v1/me", session); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("me after logout: %d", res.StatusCode)
	}
}

func TestBrowserLoginFailuresRedirectBackToTheApp(t *testing.T) {
	h := newServer(t)
	start := func() *http.Cookie { return cookie(t, browser(t, h, "GET", "/auth/login"), "homeloom_login") }

	c := start()
	for name, tc := range map[string]struct {
		path string
		want string
	}{
		"cancelled":     {"/auth/callback?error=access_denied&state=" + c.Value, "login_failed"},
		"wrong state":   {"/auth/callback?code=code-alice&state=forged", "login_failed"},
		"provider down": {"/auth/callback?code=code-down&state=" + c.Value, "auth_unavailable"},
	} {
		res := browser(t, h, "GET", tc.path, c)
		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/?auth_error="+tc.want {
			t.Errorf("%s: %d %s", name, res.StatusCode, res.Header.Get("Location"))
		}
		if len(res.cookies) != 0 {
			t.Errorf("%s: must not set a cookie", name)
		}
	}

	if res := browser(t, h, "GET", "/v1/me", &http.Cookie{Name: "homeloom_session", Value: "forged", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode}); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("forged session: %d", res.StatusCode)
	}
}

func TestHomeFlow(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	base := "/v1/households/" + created["id"].(string)

	code, home := do(t, h, "GET", base+"/home", "token-alice", "")
	if code != 200 || len(home["scenes"].([]any)) != 1 || len(home["lights"].([]any)) != 1 {
		t.Fatalf("home: %d %v", code, home)
	}
	if code, body := do(t, h, "GET", base+"/home", "token-eve", ""); code != 404 {
		t.Fatalf("non-member: %d %v", code, body)
	}
	if code, body := do(t, h, "POST", base+"/home/scenes/scene.dinner", "token-alice", ""); code != 204 {
		t.Fatalf("scene: %d %v", code, body)
	}
	if code, body := do(t, h, "POST", base+"/home/scenes/scene.party", "token-alice", ""); code != 404 {
		t.Fatalf("unknown scene: %d %v", code, body)
	}
	if code, l := do(t, h, "POST", base+"/home/lights/light.kitchen/toggle", "token-alice", ""); code != 200 || l["on"] != true {
		t.Fatalf("toggle: %d %v", code, l)
	}
	// Anything but a scene or light is refused before it reaches the core.
	for _, path := range []string{"/home/scenes/lock.front_door", "/home/lights/alarm_control_panel.home/toggle"} {
		if code, body := do(t, h, "POST", base+path, "token-alice", ""); code != 400 {
			t.Fatalf("%s: %d %v", path, code, body)
		}
	}
}
