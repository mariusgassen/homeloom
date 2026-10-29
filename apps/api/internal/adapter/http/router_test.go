package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	push := service.NewPush(store, &memory.Pusher{})
	shopping := service.NewShopping(store, store, nil)
	smart := memory.NewSmartHome([]domain.Scene{{ID: "scene.dinner", Name: "Dinner"}}, []domain.Light{{ID: "light.kitchen", Name: "Kitchen"}})
	entries := service.NewEntry(store, store, store, push, time.UTC)
	cook := service.NewCook(store, recipes, memory.Chef{}, shopping)
	home := service.NewHome(store, store, sessions, smart)
	agents := service.NewAgentAccess(store, sessions, store)
	voice := service.NewVoice(store, &memory.Speaker{})
	h, err := httpadapter.NewRouter(httpadapter.Deps{
		Version:       "1.2.3",
		PublicURL:     "https://homeloom.example.com",
		Identity:      service.NewIdentity(fakeVerifier{}, sessions, store),
		Login:         service.NewLogin(memory.NewLoginProvider(), sessions),
		Households:    service.NewHousehold(store),
		Questions:     service.NewQuestion(store, store, push),
		Entries:       entries,
		Moments:       service.NewMoments(store, store, store, push, time.UTC),
		Cook:          cook,
		Shopping:      shopping,
		Home:          home,
		Push:          push,
		Calls:         service.NewCall(store, sessions, memory.NewCallServer(), push).WithVoiceAgent("homeloom", agents).WithVoice(voice),
		Voice:         voice,
		Assistant:     service.NewAssistant(store, store, &memory.Assistant{}, entries, cook, shopping, home, time.UTC),
		Agents:        agents,
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

// doList is do for a GET that answers with an array.
func doList(t *testing.T, h http.Handler, path, token string) (int, []map[string]any) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestMcpTokensFlow(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	tokens := "/v1/households/" + created["id"].(string) + "/mcp-tokens"

	code, tok := do(t, h, "POST", tokens, "token-alice", `{"name":"Claude"}`)
	if code != http.StatusCreated || tok["name"] != "Claude" || !strings.HasPrefix(tok["secret"].(string), domain.MCPTokenPrefix) {
		t.Fatalf("create: %d %v", code, tok)
	}
	if code, body := do(t, h, "POST", tokens, "token-alice", `{"name":"   "}`); code != 400 || body["code"] != "invalid_text" {
		t.Fatalf("blank name: %d %v", code, body)
	}
	code, list := doList(t, h, tokens, "token-alice")
	if code != 200 || len(list) != 1 || list[0]["id"] != tok["id"] || list[0]["secret"] != nil {
		t.Fatalf("list: %d %v", code, list)
	}
	if code, _ := doList(t, h, tokens, "token-bob"); code != 404 {
		t.Fatalf("bob listed alice's household: %d", code)
	}
	if code, _ := do(t, h, "DELETE", "/v1/mcp-tokens/"+tok["id"].(string), "token-bob", ""); code != 404 {
		t.Fatalf("bob deleted alice's token: %d", code)
	}
	if code, _ := do(t, h, "DELETE", "/v1/mcp-tokens/"+tok["id"].(string), "token-alice", ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
}

func TestCalendarProfile(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	id := created["id"].(string)
	post := func(token, form string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/v1/households/"+id+"/calendar-profile", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := post("token-alice", "name=My+iPhone")
	body := rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/x-apple-aspen-config" {
		t.Fatalf("profile: %d %q %s", rec.Code, rec.Header().Get("Content-Type"), body)
	}
	for _, want := range []string{
		"<key>CalDAVHostName</key><string>homeloom.example.com</string>",
		"<key>CalDAVPort</key><integer>443</integer>", "<key>CalDAVUseSSL</key><true/>",
		"<key>CalDAVPrincipalURL</key><string>/dav/" + id + "/</string>",
		"<key>CalDAVPassword</key><string>" + domain.CalendarPasswordPrefix,
		"<key>PayloadType</key><string>com.apple.caldav.account</string>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("profile lacks %q:\n%s", want, body)
		}
	}
	code, list := doList(t, h, "/v1/households/"+id+"/mcp-tokens", "token-alice")
	if code != 200 || len(list) != 1 || list[0]["name"] != "My iPhone" || list[0]["access"] != "calendar" {
		t.Fatalf("list: %d %v", code, list)
	}
	if rec := post("token-bob", "name=x"); rec.Code != 404 {
		t.Fatalf("bob made a profile for alice's household: %d", rec.Code)
	}
	if rec := post("token-alice", "name="); rec.Code != 400 {
		t.Fatalf("blank name: %d", rec.Code)
	}
}

func TestUnknownPath(t *testing.T) {
	code, body := do(t, newServer(t), "GET", "/v1/nope", "token-alice", "")
	if code != http.StatusNotFound || body["code"] != "not_found" {
		t.Fatalf("got %d %v", code, body)
	}
}

func TestMomentsFlow(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	id := created["id"].(string)
	_, inv := do(t, h, "POST", "/v1/households/"+id+"/invites", "token-alice", "")
	do(t, h, "POST", "/v1/invites/"+inv["code"].(string), "token-bob", "")
	moments := "/v1/households/" + id + "/moments"
	today := time.Now().UTC().Format(time.DateOnly)

	code, trip := do(t, h, "POST", moments, "token-alice", `{"kind":"moment","text":"Lake day","visibility":"shared","day":"2026-08-02"}`)
	if code != 201 || trip["day"] != "2026-08-02" || trip["photoUrl"] != nil {
		t.Fatalf("create: %d %v", code, trip)
	}
	if code, body := do(t, h, "POST", moments, "token-alice", `{"kind":"moment","text":"Soon","visibility":"shared","day":"2999-01-01"}`); code != 400 || body["code"] != "invalid_moment_day" {
		t.Fatalf("future moment: %d %v", code, body)
	}
	do(t, h, "POST", moments, "token-alice", `{"kind":"gratitude","text":"Coffee in bed","visibility":"shared"}`)
	code, line := do(t, h, "POST", moments, "token-alice", `{"kind":"gratitude","text":"The walk","visibility":"shared","day":"2026-01-01"}`)
	if code != 201 || line["text"] != "The walk" || line["day"] != today {
		t.Fatalf("second line replaces the first, always today: %d %v", code, line)
	}
	do(t, h, "POST", moments, "token-bob", `{"kind":"moment","text":"Just mine","visibility":"private"}`)

	code, month := do(t, h, "GET", moments, "token-alice", "")
	if code != 200 || month["month"] != today[:7] || len(month["moments"].([]any)) != 1 || month["earlier"] != "2026-08" {
		t.Fatalf("this month, without bob's private moment: %d %v", code, month)
	}
	if code, body := do(t, h, "GET", moments+"?month=2026-08", "token-bob", ""); code != 200 || len(body["moments"].([]any)) != 1 {
		t.Fatalf("august: %d %v", code, body)
	}
	if code, body := do(t, h, "GET", moments+"?month=2026-13", "token-alice", ""); code != 400 {
		t.Fatalf("bad month: %d %v", code, body)
	}
	if code, _ := do(t, h, "GET", moments, "token-eve", ""); code != 404 {
		t.Fatalf("non-member: %d", code)
	}

	photo := "/v1/moments/" + trip["id"].(string) + "/photo"
	put := func(token, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest("PUT", photo, strings.NewReader(body))
		req.Header.Set("Content-Type", "image/jpeg")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	jpeg := "\xFF\xD8\xFF\xE0 a photo"
	if code, body := put("token-bob", jpeg); code != 403 || body["code"] != "not_yours" {
		t.Fatalf("someone else's moment: %d %v", code, body)
	}
	if code, body := put("token-alice", "not a jpeg"); code != 400 || body["code"] != "invalid_photo" {
		t.Fatalf("not a JPEG: %d %v", code, body)
	}
	code, withPhoto := put("token-alice", jpeg)
	url, _ := withPhoto["photoUrl"].(string)
	if code != 200 || !strings.HasPrefix(url, photo+"?v=") {
		t.Fatalf("set photo: %d %v", code, withPhoto)
	}
	req := httptest.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer token-bob")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != jpeg || rec.Header().Get("Content-Type") != "image/jpeg" || !strings.Contains(rec.Header().Get("Cache-Control"), "private") {
		t.Fatalf("get photo: %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if code, body := do(t, h, "DELETE", photo, "token-alice", ""); code != 200 || body["photoUrl"] != nil {
		t.Fatalf("remove photo: %d %v", code, body)
	}
	if code, _ := do(t, h, "GET", photo, "token-alice", ""); code != 404 {
		t.Fatalf("removed photo: %d", code)
	}

	if code, body := do(t, h, "PUT", "/v1/moments/"+line["id"].(string), "token-bob", `{"text":"Mine now","visibility":"shared"}`); code != 403 {
		t.Fatalf("change someone else's line: %d %v", code, body)
	}
	if code, body := do(t, h, "GET", "/v1/households/"+id+"/moments/resurfaced", "token-alice", ""); code != 200 || len(body["onThisDay"].([]any)) != 0 || body["gratitude"] != nil {
		t.Fatalf("nothing old enough yet: %d %v", code, body)
	}
	if code, _ := do(t, h, "DELETE", "/v1/moments/"+trip["id"].(string), "token-alice", ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
}

func TestShoppingListFlow(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	id := created["id"].(string)
	_, inv := do(t, h, "POST", "/v1/households/"+id+"/invites", "token-alice", "")
	do(t, h, "POST", "/v1/invites/"+inv["code"].(string), "token-bob", "")
	list := "/v1/households/" + id + "/shopping-list"

	if code, body := do(t, h, "POST", list+"/items", "token-alice", `{"text":"  "}`); code != 400 || body["code"] != "invalid_text" {
		t.Fatalf("blank item: %d %v", code, body)
	}
	code, milk := do(t, h, "POST", list+"/items", "token-alice", `{"text":" Milk "}`)
	if code != 201 || milk["text"] != "Milk" || milk["done"] != false || milk["recipe"] != nil {
		t.Fatalf("add: %d %v", code, milk)
	}
	do(t, h, "POST", list+"/items", "token-bob", `{"text":"Bread"}`)
	item := list + "/items/" + milk["id"].(string)

	if code, _ := doList(t, h, list, "token-eve"); code != 404 {
		t.Fatalf("non-member sees the list: %d", code)
	}
	if code, body := do(t, h, "PUT", item+"/done", "token-eve", ""); code != 404 {
		t.Fatalf("non-member ticks: %d %v", code, body)
	}
	if code, body := do(t, h, "PUT", item+"/done", "token-bob", ""); code != 200 || body["done"] != true || body["doneAt"] == nil {
		t.Fatalf("tick: %d %v", code, body)
	}
	if code, items := doList(t, h, list, "token-alice"); code != 200 || len(items) != 2 || items[0]["text"] != "Bread" || items[1]["done"] != true {
		t.Fatalf("list: %d %v", code, items)
	}
	if code, body := do(t, h, "DELETE", item+"/done", "token-alice", ""); code != 200 || body["done"] != false {
		t.Fatalf("untick: %d %v", code, body)
	}
	do(t, h, "PUT", item+"/done", "token-alice", "")
	if code, _ := do(t, h, "DELETE", list+"/done", "token-alice", ""); code != 204 {
		t.Fatalf("clear: %d", code)
	}
	code, items := doList(t, h, list, "token-bob")
	if code != 200 || len(items) != 1 || items[0]["text"] != "Bread" {
		t.Fatalf("after clearing: %d %v", code, items)
	}
	if code, _ := do(t, h, "DELETE", list+"/items/"+items[0]["id"].(string), "token-alice", ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := do(t, h, "DELETE", item, "token-alice", ""); code != 404 {
		t.Fatalf("delete a cleared item: %d", code)
	}
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

	code, added := do(t, h, "POST", base+"/shopping-list", "token-alice", `{"recipe":"green-curry","ingredientIds":["i2"]}`, "Accept-Language", "de")
	if code != 200 || added["added"] != 1.0 || added["list"] != "Einkaufsliste" {
		t.Fatalf("add: %d %v", code, added)
	}
	if code, items := doList(t, h, base+"/shopping-list", "token-alice"); code != 200 || len(items) != 1 || items[0]["recipe"] != "Green curry" {
		t.Fatalf("list after adding a recipe: %d %v", code, items)
	}
	if code, body := do(t, h, "POST", base+"/shopping-list", "token-alice", `{"recipe":"green-curry","ingredientIds":["x"]}`); code != 400 || body["code"] != "invalid_ingredients" {
		t.Fatalf("foreign ingredient: %d %v", code, body)
	}
}

func TestCookingWithoutMealie(t *testing.T) {
	h := newServerWith(t, nil)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	code, body := do(t, h, "POST", "/v1/households/"+created["id"].(string)+"/cook/suggestions", "token-alice", `{"mood":"cozy"}`, "Accept-Language", "de")
	if code != 503 || body["code"] != "cooking_unavailable" || body["message"] != "Kochen geht gerade nicht." {
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

	if code, home := do(t, h, "GET", base+"/home", "token-alice", ""); code != 200 || home["connected"] != false {
		t.Fatalf("before connecting: %d %v", code, home)
	}
	code, c := do(t, h, "POST", base+"/home/connection", "token-alice", "")
	if code != 200 {
		t.Fatalf("connect: %d %v", code, c)
	}
	to, _ := url.Parse(c["url"].(string))
	callback := "/auth/homeassistant/callback?code=good&state=" + url.QueryEscape(to.Query().Get("state"))
	if code, loc := redirect(t, h, callback, "token-alice"); code != 302 || loc != "/home" {
		t.Fatalf("callback: %d %s", code, loc)
	}
	if _, loc := redirect(t, h, callback, "token-alice"); loc != "/home?home_error=connect_failed" {
		t.Fatalf("state reused: %s", loc)
	}

	code, home := do(t, h, "GET", base+"/home", "token-alice", "")
	if code != 200 || home["connected"] != true || len(home["scenes"].([]any)) != 1 || len(home["lights"].([]any)) != 1 {
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

	if code, body := do(t, h, "DELETE", base+"/home/connection", "token-alice", ""); code != 204 {
		t.Fatalf("disconnect: %d %v", code, body)
	}
	if code, body := do(t, h, "POST", base+"/home/scenes/scene.dinner", "token-alice", "", "Accept-Language", "de"); code != 503 || body["code"] != "home_unavailable" {
		t.Fatalf("after disconnect: %d %v", code, body)
	}
}

// redirect sends a GET and returns the status and where it redirects to.
func redirect(t *testing.T, h http.Handler, path, token string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Header().Get("Location")
}

func TestPushFlow(t *testing.T) {
	h := newServer(t)
	if code, body := do(t, h, "GET", "/v1/push", "token-alice", ""); code != 200 || body["publicKey"] != "test-public-key" {
		t.Fatalf("config: %d %v", code, body)
	}
	sub := func(endpoint string) string {
		return `{"endpoint":"` + endpoint + `","keys":{"p256dh":"p","auth":"a"}}`
	}
	if code, body := do(t, h, "POST", "/v1/push/subscriptions", "token-alice", sub("https://web.push.apple.com/abc")); code != 204 {
		t.Fatalf("subscribe: %d %v", code, body)
	}
	if code, body := do(t, h, "POST", "/v1/push/subscriptions", "token-alice", sub("https://example.com/hook")); code != 400 || body["code"] != "invalid" {
		t.Fatalf("foreign endpoint: %d %v", code, body)
	}
	if code, body := do(t, h, "POST", "/v1/push/subscriptions", "token-alice", `{"endpoint":"https://web.push.apple.com/abc"}`); code != 400 {
		t.Fatalf("missing keys: %d %v", code, body)
	}
	if code, body := do(t, h, "DELETE", "/v1/push/subscriptions?endpoint=https%3A%2F%2Fweb.push.apple.com%2Fabc", "token-alice", ""); code != 204 {
		t.Fatalf("unsubscribe: %d %v", code, body)
	}
	if code, _ := do(t, h, "POST", "/v1/push/subscriptions", "", sub("https://web.push.apple.com/abc")); code != 401 {
		t.Fatalf("needs auth: %d", code)
	}
}

func TestVoiceFlow(t *testing.T) {
	h := newServer(t)
	code, got := do(t, h, "GET", "/v1/me/voice", "token-alice", "")
	settings, _ := got["settings"].(map[string]any)
	if code != 200 || got["available"] != true || settings["voice"] != "coral" || settings["speed"] != 1.0 || settings["tone"] != "warm" {
		t.Fatalf("defaults: %d %v", code, got)
	}
	if code, got := do(t, h, "PUT", "/v1/me/voice", "token-alice", `{"voice":"marin","speed":1.25,"tone":"calm"}`); code != 200 || got["voice"] != "marin" {
		t.Fatalf("save: %d %v", code, got)
	}
	if _, got := do(t, h, "GET", "/v1/me/voice", "token-alice", ""); got["settings"].(map[string]any)["tone"] != "calm" {
		t.Fatalf("saved: %v", got)
	}
	if code, _ := do(t, h, "PUT", "/v1/me/voice", "token-alice", `{"voice":"marin","speed":3,"tone":"calm"}`); code != 400 {
		t.Fatalf("too fast: %d", code)
	}

	req := httptest.NewRequest("POST", "/v1/me/voice/preview", strings.NewReader(`{"voice":"cedar","speed":1,"tone":"playful"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token-alice")
	req.Header.Set("Accept-Language", "de")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "audio/mpeg" || !strings.HasPrefix(rec.Body.String(), "cedar:Hi, ich bin Homeloom") {
		t.Fatalf("preview: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}

func TestCallFlow(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	id := created["id"].(string)
	_, inv := do(t, h, "POST", "/v1/households/"+id+"/invites", "token-alice", "")
	do(t, h, "POST", "/v1/invites/"+inv["code"].(string), "token-bob", "")
	_, bob := do(t, h, "GET", "/v1/me", "token-bob", "")
	_, alice := do(t, h, "GET", "/v1/me", "token-alice", "")
	invite := func(u map[string]any) string { return `{"invite":["` + u["id"].(string) + `"]}` }
	calls := "/v1/households/" + id + "/calls"

	code, c := do(t, h, "POST", calls, "token-alice", invite(bob))
	if code != 201 || c["url"] != "wss://calls.example" || c["token"] == "" || c["id"] == "" {
		t.Fatalf("start: %d %v", code, c)
	}
	call := calls + "/" + c["id"].(string)
	if code, answer := do(t, h, "POST", call+"/join", "token-bob", ""); code != 200 || answer["room"] != c["room"] {
		t.Fatalf("answer: %d %v", code, answer)
	}
	if code, _ := do(t, h, "POST", call+"/invites", "token-bob", invite(alice)); code != 204 {
		t.Fatalf("invite: %d", code)
	}
	if code, body := do(t, h, "POST", calls, "token-alice", invite(alice)); code != 400 || body["code"] != "invalid" {
		t.Fatalf("yourself: %d %v", code, body)
	}
	if code, body := do(t, h, "POST", call+"/join", "token-eve", ""); code != 404 || body["code"] != "not_found" {
		t.Fatalf("outsider: %d %v", code, body)
	}
	if code, _ := do(t, h, "POST", call+"/join", "", ""); code != 401 {
		t.Fatalf("needs auth: %d", code)
	}

	if code, body := do(t, h, "POST", calls+"/homeloom", "token-alice", "", "Accept-Language", "de"); code != 201 || !strings.HasPrefix(body["room"].(string), "homeloom-") {
		t.Fatalf("talk to homeloom: %d %v", code, body)
	}
	if code, _ := do(t, h, "POST", calls+"/homeloom", "token-eve", ""); code != 404 {
		t.Fatalf("outsider talking to homeloom: %d", code)
	}

	code, link := do(t, h, "POST", call+"/guest-links", "token-alice", "")
	if code != 201 || link["token"] == "" || link["expiresAt"] == "" {
		t.Fatalf("guest link: %d %v", code, link)
	}
	guest := "/v1/guest-calls/" + link["token"].(string)
	if code, body := do(t, h, "GET", guest, "", ""); code != 200 || body["invitedBy"] != "alice" {
		t.Fatalf("guest call: %d %v", code, body)
	}
	if code, body := do(t, h, "POST", guest, "", `{"name":"Grandma","device":"phone-0123456789abcdef"}`); code != 200 || body["room"] != c["room"] {
		t.Fatalf("guest join: %d %v", code, body)
	}
	if code, body := do(t, h, "POST", guest, "", `{"name":"Mallory","device":"laptop-0123456789abcdef"}`, "Accept-Language", "de"); code != 409 || body["code"] != "guest_link_used" || body["message"] == "" {
		t.Fatalf("forwarded link: %d %v", code, body)
	}
	if code, _ := do(t, h, "POST", guest, "", `{"name":"","device":"phone-0123456789abcdef"}`); code != 400 {
		t.Fatalf("guest without a name: %d", code)
	}
	if code, _ := do(t, h, "GET", "/v1/guest-calls/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "", ""); code != 404 {
		t.Fatalf("unknown guest link: %d", code)
	}
}

func TestEntriesFlow(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	id := created["id"].(string)
	_, inv := do(t, h, "POST", "/v1/households/"+id+"/invites", "token-alice", "")
	do(t, h, "POST", "/v1/invites/"+inv["code"].(string), "token-bob", "")
	entries := "/v1/households/" + id + "/entries"

	if code, body := do(t, h, "POST", entries, "token-alice", `{"kind":"reminder","text":"Bins","visibility":"reveal","revealAt":"2030-01-01T00:00:00Z"}`, "Accept-Language", "de"); code != 400 || body["code"] != "invalid_reveal" || !strings.Contains(body["message"].(string), "Notizen") {
		t.Fatalf("reminder revealed later: %d %v", code, body)
	}
	if code, body := do(t, h, "POST", entries, "token-alice", `{"kind":"reminder","text":"`+strings.Repeat("x", 201)+`","visibility":"shared"}`); code != 400 || body["code"] != "invalid_text" || !strings.Contains(body["message"].(string), "200") {
		t.Fatalf("long reminder: %d %v", code, body)
	}
	if code, body := do(t, h, "POST", entries, "token-alice", `{"kind":"task","text":"x","visibility":"shared"}`); code != 400 {
		t.Fatalf("unknown kind: %d %v", code, body)
	}
	code, r := do(t, h, "POST", entries, "token-alice", `{"kind":"reminder","text":"Bins","visibility":"shared","dueAt":"2030-01-01T07:00:00Z","repeat":"weekly"}`)
	if code != 201 || r["text"] != "Bins" || r["sealed"] != false || r["repeat"] != "weekly" {
		t.Fatalf("create reminder: %d %v", code, r)
	}
	code, letter := do(t, h, "POST", entries, "token-alice", `{"kind":"note","text":"Love you","visibility":"reveal","revealAt":"2030-01-01T00:00:00Z"}`)
	if code != 201 {
		t.Fatalf("create letter: %d %v", code, letter)
	}
	_, secret := do(t, h, "POST", entries, "token-alice", `{"kind":"note","text":"Gift ideas","visibility":"private"}`)

	if code, body := do(t, h, "GET", "/v1/entries/"+letter["id"].(string), "token-bob", ""); code != 200 || body["sealed"] != true || body["text"] != nil {
		t.Fatalf("bob sees a sealed letter: %d %v", code, body)
	}
	if code, _ := do(t, h, "GET", "/v1/entries/"+secret["id"].(string), "token-bob", ""); code != 404 {
		t.Fatalf("private note to bob: want 404, got %d", code)
	}
	if code, body := do(t, h, "PUT", "/v1/entries/"+letter["id"].(string), "token-bob", `{"text":"x","visibility":"reveal","revealAt":"2030-01-01T00:00:00Z"}`); code != 403 || body["code"] != "not_yours" {
		t.Fatalf("bob edits the letter: %d %v", code, body)
	}
	if code, _ := do(t, h, "GET", entries, "token-eve", ""); code != 404 {
		t.Fatalf("non-member list: want 404, got %d", code)
	}

	req := httptest.NewRequest("GET", entries+"?kind=note", nil)
	req.Header.Set("Authorization", "Bearer token-bob")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var notes []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &notes); err != nil || len(notes) != 1 || notes[0]["id"] != letter["id"] {
		t.Fatalf("bob's notes: %v %s", err, rec.Body)
	}

	done := "/v1/entries/" + r["id"].(string) + "/done"
	if code, body := do(t, h, "PUT", done, "token-bob", ""); code != 200 || body["dueAt"] != "2030-01-08T07:00:00Z" || body["doneAt"] != nil {
		t.Fatalf("ticking off a weekly reminder moves it on: %d %v", code, body)
	}
	if code, body := do(t, h, "PUT", "/v1/entries/"+letter["id"].(string)+"/done", "token-alice", ""); code != 409 || body["code"] != "not_a_reminder" {
		t.Fatalf("tick off a note: %d %v", code, body)
	}
	if code, body := do(t, h, "PUT", "/v1/entries/"+r["id"].(string), "token-bob", `{"text":"Bins out","visibility":"shared","dueAt":"2030-01-08T07:00:00Z"}`); code != 200 || body["text"] != "Bins out" || body["repeat"] != "never" {
		t.Fatalf("bob edits the shared reminder: %d %v", code, body)
	}
	if code, body := do(t, h, "PUT", done, "token-bob", ""); code != 200 || body["doneAt"] == nil || body["doneBy"] == nil {
		t.Fatalf("tick off: %d %v", code, body)
	}
	if code, body := do(t, h, "DELETE", done, "token-alice", ""); code != 200 || body["doneAt"] != nil {
		t.Fatalf("reopen: %d %v", code, body)
	}
	if code, _ := do(t, h, "DELETE", "/v1/entries/"+r["id"].(string), "token-bob", ""); code != 204 {
		t.Fatalf("delete: want 204, got %d", code)
	}
	if code, _ := do(t, h, "GET", "/v1/entries/"+r["id"].(string), "token-alice", ""); code != 404 {
		t.Fatalf("after delete: want 404, got %d", code)
	}
}

func TestCalendarFlow(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	id := created["id"].(string)
	entries := "/v1/households/" + id + "/entries"

	if code, body := do(t, h, "POST", entries, "token-alice", `{"kind":"event","text":"Dinner","visibility":"shared"}`); code != 400 || body["code"] != "invalid_event_time" {
		t.Fatalf("event without a start: %d %v", code, body)
	}
	code, ev := do(t, h, "POST", entries, "token-alice", `{"kind":"event","text":"Dinner, with Jo","visibility":"shared","startsAt":"2030-01-01T18:00:00Z","endsAt":"2030-01-01T21:00:00Z","alertMinutes":60}`)
	if code != 201 || ev["dueAt"] != "2030-01-01T17:00:00Z" || ev["allDay"] != false {
		t.Fatalf("create event: %d %v", code, ev)
	}
	do(t, h, "POST", entries, "token-alice", `{"kind":"event","text":"Ghent","visibility":"private","allDay":true,"startsAt":"2030-02-01T12:00:00Z","endsAt":"2030-02-02T12:00:00Z"}`)

	feed := "/v1/households/" + id + "/calendar-feed"
	if code, body := do(t, h, "GET", feed, "token-alice", ""); code != 200 || body["active"] != false {
		t.Fatalf("no link yet: %d %v", code, body)
	}
	code, link := do(t, h, "POST", feed, "token-alice", "")
	path, _ := link["path"].(string)
	if code != 201 || !strings.HasPrefix(path, "/v1/calendar-feeds/") {
		t.Fatalf("create link: %d %v", code, link)
	}
	if code, _ := do(t, h, "POST", feed, "token-eve", ""); code != 404 {
		t.Fatalf("non-member link: want 404, got %d", code)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	ics := rec.Body.String()
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/calendar") {
		t.Fatalf("feed: %d %s", rec.Code, rec.Header())
	}
	for _, want := range []string{"BEGIN:VCALENDAR\r\n", "SUMMARY:Dinner\\, with Jo\r\n", "DTSTART:20300101T180000Z\r\n", "DTEND:20300101T210000Z\r\n", "DTSTART;VALUE=DATE:20300201\r\n", "DTEND;VALUE=DATE:20300203\r\n", "CLASS:PRIVATE\r\n"} {
		if !strings.Contains(ics, want) {
			t.Fatalf("feed lacks %q:\n%s", want, ics)
		}
	}

	if code, _ := do(t, h, "DELETE", feed, "token-alice", ""); code != 204 {
		t.Fatalf("turn off: want 204, got %d", code)
	}
	if code, _ := do(t, h, "GET", path, "", ""); code != 404 {
		t.Fatalf("after turning off: want 404, got %d", code)
	}
}

// chat posts a message and returns the server-sent events of the answer by name.
func chat(t *testing.T, h http.Handler, household, token, body string) (int, map[string][]map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/households/"+household+"/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		return rec.Code, nil
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	events := map[string][]map[string]any{}
	for _, block := range strings.Split(strings.TrimSpace(rec.Body.String()), "\n\n") {
		name, data, _ := strings.Cut(block, "\n")
		var v map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "data: ")), &v); err != nil {
			t.Fatalf("event %q: %v", block, err)
		}
		events[strings.TrimPrefix(name, "event: ")] = append(events[strings.TrimPrefix(name, "event: ")], v)
	}
	return 200, events
}

func TestAssistantFlow(t *testing.T) {
	h := newServer(t)
	_, created := do(t, h, "POST", "/v1/households", "token-alice", `{"name":"Home"}`)
	id := created["id"].(string)

	code, events := chat(t, h, id, "token-alice", `{"text":"Hallo"}`)
	if code != 200 || len(events["conversation"]) != 1 || events["text"][0]["text"] != "en: Hallo" || events["done"][0]["text"] != "en: Hallo" {
		t.Fatalf("chat: %d %v", code, events)
	}
	conv := events["conversation"][0]["id"].(string)
	if code, events := chat(t, h, id, "token-alice", `{"conversationId":"`+conv+`","text":"/get_home {}"}`); code != 200 || events["tool"][0]["name"] != "get_home" {
		t.Fatalf("tool: %d %v", code, events)
	}
	if code, events := chat(t, h, id, "token-alice", `{"conversationId":"`+conv+`","text":"down"}`); code != 200 || events["error"][0]["code"] != "assistant_unavailable" || events["done"] != nil {
		t.Fatalf("down: %d %v", code, events)
	}

	code, body := do(t, h, "GET", "/v1/conversations/"+conv, "token-alice", "")
	if msgs, _ := body["messages"].([]any); code != 200 || len(msgs) != 5 || body["title"] != "Hallo" {
		t.Fatalf("get: %d %v", code, body)
	}
	req := httptest.NewRequest("GET", "/v1/households/"+id+"/conversations", nil)
	req.Header.Set("Authorization", "Bearer token-alice")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), conv) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}

	if code, _ := chat(t, h, id, "token-alice", `{"text":"`+strings.Repeat("x", 2001)+`"}`); code != 400 {
		t.Fatalf("too long: want 400, got %d", code)
	}
	if code, _ := chat(t, h, id, "token-eve", `{"text":"hi"}`); code != 404 {
		t.Fatalf("non-member: want 404, got %d", code)
	}
	if code, _ := do(t, h, "GET", "/v1/conversations/"+conv, "token-eve", ""); code != 404 {
		t.Fatalf("someone else's chat: want 404, got %d", code)
	}
	if code, _ := do(t, h, "DELETE", "/v1/conversations/"+conv, "token-alice", ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
}

// TestDAVMethodsReachTheirHandlers guards against the router answering WebDAV and CalDAV methods
// with 405 before /dav and /webdav see them.
func TestDAVMethodsReachTheirHandlers(t *testing.T) {
	var got []string
	record := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusMultiStatus)
	})
	h, err := httpadapter.NewRouter(httpadapter.Deps{CalDAV: record, Notes: record})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/.well-known/caldav", "/dav", "/dav/x/calendars/events/", "/webdav", "/webdav/a.md"} {
		for _, method := range []string{"PROPFIND", "PROPPATCH", "REPORT", "MKCALENDAR", "MKCOL", "MOVE", "COPY", "LOCK", "UNLOCK", "OPTIONS", "PUT"} {
			got = nil
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
			if rec.Code != http.StatusMultiStatus || len(got) != 1 {
				t.Errorf("%s %s: %d, handler saw %v", method, path, rec.Code, got)
			}
		}
	}
}
