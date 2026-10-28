package notesdav_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/notesdav"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

type fixture struct {
	url, password string
	entries       *service.Entry
	agents        *service.AgentAccess
	alice         domain.User
	household     domain.Household
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	store := memory.NewStore()
	entries := service.NewEntry(store, store, store, service.NewPush(store, &memory.Pusher{}), time.UTC)
	agents := service.NewAgentAccess(store, memory.NewSessions(), store)
	alice, err := store.UpsertByIdentity(ctx, domain.Identity{Subject: "alice", Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := service.NewHousehold(store).Create(ctx, alice, "Home")
	if err != nil {
		t.Fatal(err)
	}
	_, password, err := agents.CreateToken(ctx, alice, m.Household.ID, "iPhone", domain.AccessNotes)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(notesdav.NewHandler(notesdav.Deps{Agents: agents, Entries: entries}))
	t.Cleanup(srv.Close)
	return &fixture{url: srv.URL, password: password, entries: entries, agents: agents, alice: alice, household: m.Household}
}

// reply is what a request got back; the body is already read and closed.
type reply struct {
	StatusCode int
	Header     http.Header
}

func (f *fixture) do(t *testing.T, method, path, password, body string, header ...string) (reply, string) {
	t.Helper()
	req, err := http.NewRequest(method, f.url+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if password != "" {
		req.SetBasicAuth("alice", password)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return reply{res.StatusCode, res.Header}, string(b)
}

const propfind = `<?xml version="1.0"?><propfind xmlns="DAV:"><prop><resourcetype/><getetag/><getcontentlength/></prop></propfind>`

func TestSignIn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, calendar, err := f.agents.CreateToken(ctx, f.alice, f.household.ID, "iPhone", domain.AccessCalendar)
	if err != nil {
		t.Fatal(err)
	}
	for _, password := range []string{"", "wrong", calendar, domain.NotesPasswordPrefix + "made-up"} {
		res, _ := f.do(t, "PROPFIND", "/webdav/", password, propfind, "Depth", "0", "Content-Type", "application/xml")
		if res.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(res.Header.Get("WWW-Authenticate"), "Basic") {
			t.Fatalf("password %q: %d", password, res.StatusCode)
		}
	}
	res, body := f.do(t, "PROPFIND", "/webdav/", f.password, propfind, "Depth", "1", "Content-Type", "application/xml")
	if res.StatusCode != http.StatusMultiStatus || !strings.Contains(body, "/webdav/Shared/") || !strings.Contains(body, "/webdav/Private/") {
		t.Fatalf("root: %d %s", res.StatusCode, body)
	}
}

func TestNotes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// Notes made in Homeloom are named after their first line; "later" notes stay out.
	shared, err := f.entries.Create(ctx, f.alice, f.household.ID, domain.KindNote, domain.EntryFields{Visibility: domain.VisibilityShared, Text: "# Wifi\nhomeloom / 1234"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.entries.Create(ctx, f.alice, f.household.ID, domain.KindNote, domain.EntryFields{Visibility: domain.VisibilityShared, Text: "Wifi"}); err != nil {
		t.Fatal(err)
	}
	reveal := time.Now().Add(24 * time.Hour)
	if _, err := f.entries.Create(ctx, f.alice, f.household.ID, domain.KindNote, domain.EntryFields{Visibility: domain.VisibilityReveal, RevealAt: &reveal, Text: "Surprise"}); err != nil {
		t.Fatal(err)
	}
	res, body := f.do(t, "PROPFIND", "/webdav/Shared/", f.password, propfind, "Depth", "1", "Content-Type", "application/xml")
	if res.StatusCode != http.StatusMultiStatus || !strings.Contains(body, "/webdav/Shared/Wifi.md") || !strings.Contains(body, "/webdav/Shared/Wifi%202.md") ||
		strings.Contains(body, "Surprise") {
		t.Fatalf("shared: %d %s", res.StatusCode, body)
	}
	res, body = f.do(t, "GET", "/webdav/Shared/Wifi.md", f.password, "")
	if res.StatusCode != http.StatusOK || body != shared.Text || res.Header.Get("ETag") == "" {
		t.Fatalf("get: %d %q", res.StatusCode, body)
	}
	// Once an app writes it, it keeps its name though its first line changes.
	if res, _ := f.do(t, "PUT", "/webdav/Shared/Wifi.md", f.password, "Network\nhomeloom / 1234", "If-Match", res.Header.Get("ETag")); res.StatusCode/100 != 2 {
		t.Fatalf("edit: %d", res.StatusCode)
	}
	if res, body := f.do(t, "GET", "/webdav/Shared/Wifi.md", f.password, ""); res.StatusCode != http.StatusOK || body != "Network\nhomeloom / 1234" {
		t.Fatalf("edited: %d %q", res.StatusCode, body)
	}

	// A note an app makes keeps its name; an edit needs the current ETag when the app sends one.
	if res, _ := f.do(t, "PUT", "/webdav/Private/Ideas.md", f.password, "\xef\xbb\xbfA garden", "If-None-Match", "*"); res.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", res.StatusCode)
	}
	res, body = f.do(t, "GET", "/webdav/Private/Ideas.md", f.password, "")
	if res.StatusCode != http.StatusOK || body != "A garden" {
		t.Fatalf("created: %d %q", res.StatusCode, body)
	}
	tag := res.Header.Get("ETag")
	if res, _ := f.do(t, "PUT", "/webdav/Private/Ideas.md", f.password, "Stale", "If-Match", `"stale"`); res.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("stale edit: %d", res.StatusCode)
	}
	if res, _ := f.do(t, "PUT", "/webdav/Private/Ideas.md", f.password, "A garden\nwith herbs", "If-Match", tag); res.StatusCode/100 != 2 {
		t.Fatalf("edit: %d", res.StatusCode)
	}
	for _, p := range []string{"/webdav/Private/.hidden.md", "/webdav/Private/photo.jpg", "/webdav/Private/sub/a.md", "/webdav/a.md"} {
		if res, _ := f.do(t, "PUT", p, f.password, "x"); res.StatusCode != http.StatusForbidden {
			t.Fatalf("put %s: %d", p, res.StatusCode)
		}
	}
	if res, _ := f.do(t, "PUT", "/webdav/Private/Long.md", f.password, strings.Repeat("x", domain.MaxNoteText+1)); res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("too long: %d", res.StatusCode)
	}

	// Moving a note to Shared shares it, under its new name.
	if res, _ := f.do(t, "MOVE", "/webdav/Private/Ideas.md", f.password, "", "Destination", f.url+"/webdav/Shared/Garden.md"); res.StatusCode != http.StatusCreated {
		t.Fatalf("move: %d", res.StatusCode)
	}
	if res, _ := f.do(t, "GET", "/webdav/Private/Ideas.md", f.password, ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("moved away: %d", res.StatusCode)
	}
	e, err := f.entries.SyncEntry(ctx, f.alice, f.household.ID, domain.SyncList{Kind: domain.KindNote, Visibility: domain.VisibilityShared}, "Garden.md")
	if err != nil || e.Text != "A garden\nwith herbs" {
		t.Fatalf("moved: %+v, %v", e, err)
	}

	if res, _ := f.do(t, "DELETE", "/webdav/Shared/Garden.md", f.password, ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	if _, err := f.entries.Get(ctx, f.alice, e.ID); err == nil {
		t.Fatal("deleted note is still there")
	}
	if res, _ := f.do(t, "MKCOL", "/webdav/Recipes/", f.password, ""); res.StatusCode != http.StatusForbidden {
		t.Fatalf("mkcol: %d", res.StatusCode)
	}
}
