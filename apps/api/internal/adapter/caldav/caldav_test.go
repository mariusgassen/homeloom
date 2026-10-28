package caldav_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/caldav"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

type fixture struct {
	url, password, home string
	entries             *service.Entry
	alice               domain.User
	household           domain.Household
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	store := memory.NewStore()
	entries := service.NewEntry(store, store, store, service.NewPush(store, &memory.Pusher{}), berlin)
	agents := service.NewAgentAccess(store, memory.NewSessions(), store)
	alice, err := store.UpsertByIdentity(ctx, domain.Identity{Subject: "alice", Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := service.NewHousehold(store).Create(ctx, alice, "Home")
	if err != nil {
		t.Fatal(err)
	}
	_, password, err := agents.CreateToken(ctx, alice, m.Household.ID, "iPhone", domain.AccessCalendar)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(caldav.NewHandler(caldav.Deps{Agents: agents, Entries: entries, Location: berlin}))
	t.Cleanup(srv.Close)
	return &fixture{
		url: srv.URL, password: password, home: "/dav/" + m.Household.ID.String() + "/calendars/",
		entries: entries, alice: alice, household: m.Household,
	}
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
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return reply{res.StatusCode, res.Header}, string(b)
}

const propfind = `<?xml version="1.0"?><propfind xmlns="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><prop><current-user-principal/><c:calendar-home-set/><displayname/><resourcetype/><c:supported-calendar-component-set/><getetag/></prop></propfind>`

func TestSignIn(t *testing.T) {
	f := newFixture(t)
	if res, _ := f.do(t, "GET", "/.well-known/caldav", "", ""); res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != "/dav/" {
		t.Fatalf("well-known: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	for _, password := range []string{"", "wrong", domain.CalendarPasswordPrefix + "made-up"} {
		res, _ := f.do(t, "PROPFIND", "/dav/", password, propfind, "Depth", "0", "Content-Type", "application/xml")
		if res.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(res.Header.Get("WWW-Authenticate"), "Basic") {
			t.Fatalf("password %q: %d", password, res.StatusCode)
		}
	}
	res, body := f.do(t, "PROPFIND", "/dav/", f.password, propfind, "Depth", "0", "Content-Type", "application/xml")
	if res.StatusCode != http.StatusMultiStatus || !strings.Contains(body, "/dav/"+f.household.ID.String()+"/") {
		t.Fatalf("root: %d %s", res.StatusCode, body)
	}
}

func TestListsAndObjects(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	xml := []string{"Content-Type", "application/xml", "Accept-Language", "de"}

	res, body := f.do(t, "PROPFIND", f.home, f.password, propfind, append(xml, "Depth", "1")...)
	if res.StatusCode != http.StatusMultiStatus {
		t.Fatalf("home: %d %s", res.StatusCode, body)
	}
	for _, want := range []string{"events/", "events-private/", "reminders/", "reminders-private/", "VTODO", "VEVENT"} {
		if !strings.Contains(body, want) {
			t.Fatalf("home lacks %q: %s", want, body)
		}
	}

	// An event the app makes, in Berlin time, with an alert 15 minutes before.
	event := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//iOS 26//EN\r\nBEGIN:VEVENT\r\nUID:E1\r\nDTSTAMP:20261028T100000Z\r\n" +
		"DTSTART;TZID=Europe/Berlin:20261105T190000\r\nDTEND;TZID=Europe/Berlin:20261105T210000\r\nSUMMARY:Dinner at Anna's\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=TH\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nDESCRIPTION:x\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	ics := []string{"Content-Type", "text/calendar; charset=utf-8"}
	res, _ = f.do(t, "PUT", f.home+"events/E1.ics", f.password, event, append(ics, "If-None-Match", "*")...)
	if res.StatusCode != http.StatusCreated || res.Header.Get("ETag") == "" {
		t.Fatalf("put: %d", res.StatusCode)
	}
	tag := res.Header.Get("ETag")
	if res, _ := f.do(t, "PUT", f.home+"events/E1.ics", f.password, event, append(ics, "If-None-Match", "*")...); res.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("second create: %d", res.StatusCode)
	}
	if res, _ := f.do(t, "PUT", f.home+"events/E1.ics", f.password, event, append(ics, "If-Match", `"stale"`)...); res.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("stale update: %d", res.StatusCode)
	}

	e, err := f.entries.SyncEntry(ctx, f.alice, f.household.ID, domain.SyncList{Kind: domain.KindEvent, Visibility: domain.VisibilityShared}, "E1.ics")
	if err != nil {
		t.Fatal(err)
	}
	if !e.StartsAt.Equal(time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC)) || !e.EndsAt.Equal(time.Date(2026, 11, 5, 20, 0, 0, 0, time.UTC)) ||
		e.Text != "Dinner at Anna's" || e.Repeat != domain.RepeatWeekly || e.AlertMinutes == nil || *e.AlertMinutes != 15 {
		t.Fatalf("stored: %+v", e)
	}

	res, body = f.do(t, "GET", f.home+"events/E1.ics", f.password, "")
	if res.StatusCode != http.StatusOK || res.Header.Get("ETag") != tag {
		t.Fatalf("get: %d %q, want %q", res.StatusCode, res.Header.Get("ETag"), tag)
	}
	for _, want := range []string{"UID:E1", "DTSTART;TZID=Europe/Berlin:20261105T190000", "RRULE:FREQ=WEEKLY", "TRIGGER:-PT15M", "SUMMARY:Dinner at Anna's"} {
		if !strings.Contains(body, want) {
			t.Fatalf("get lacks %q:\n%s", want, body)
		}
	}
	if res, _ := f.do(t, "GET", f.home+"events-private/E1.ics", f.password, ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("event in the wrong list: %d", res.StatusCode)
	}
	if res, _ := f.do(t, "GET", "/dav/00000000-0000-0000-0000-000000000000/calendars/events/E1.ics", f.password, ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("another household's path: %d", res.StatusCode)
	}

	// A reminder made in Homeloom shows in the reminders list; ticking it off in the app ticks it off.
	due := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	r, err := f.entries.Create(ctx, f.alice, f.household.ID, domain.KindReminder, domain.EntryFields{Visibility: domain.VisibilityShared, Text: "Bins", DueAt: &due})
	if err != nil {
		t.Fatal(err)
	}
	query := `<?xml version="1.0"?><c:calendar-query xmlns="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><prop><getetag/><c:calendar-data/></prop>` +
		`<c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VTODO"/></c:comp-filter></c:filter></c:calendar-query>`
	res, body = f.do(t, "REPORT", f.home+"reminders/", f.password, query, append(xml, "Depth", "1")...)
	if res.StatusCode != http.StatusMultiStatus || !strings.Contains(body, r.ID.String()+".ics") || !strings.Contains(body, "SUMMARY:Bins") {
		t.Fatalf("report: %d %s", res.StatusCode, body)
	}
	done := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:x\r\nBEGIN:VTODO\r\nUID:" + r.ID.String() + "@homeloom\r\nDTSTAMP:20261028T100000Z\r\n" +
		"SUMMARY:Bins\r\nDUE;VALUE=DATE:20261101\r\nSTATUS:COMPLETED\r\nCOMPLETED:20261028T100000Z\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	if res, body := f.do(t, "PUT", f.home+"reminders/"+r.ID.String()+".ics", f.password, done, ics...); res.StatusCode != http.StatusCreated {
		t.Fatalf("tick off: %d %s", res.StatusCode, body)
	}
	if r, err = f.entries.Get(ctx, f.alice, r.ID); err != nil || r.DoneAt == nil || !r.DueAt.Equal(time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("ticked off: %+v, %v", r, err)
	}

	if res, _ := f.do(t, "DELETE", f.home+"events/E1.ics", f.password, ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	if res, _ := f.do(t, "GET", f.home+"events/E1.ics", f.password, ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete: %d", res.StatusCode)
	}
	if res, _ := f.do(t, "PROPPATCH", f.home+"events/", f.password, `<?xml version="1.0"?><propertyupdate xmlns="DAV:"><set><prop><x:calendar-color xmlns:x="http://apple.com/ns/ical/">#FF0000</x:calendar-color></prop></set></propertyupdate>`, xml...); res.StatusCode != http.StatusMultiStatus {
		t.Fatalf("proppatch: %d", res.StatusCode)
	}
}

// TestDiscovery follows the requests iOS makes to add an account and sync it.
func TestDiscovery(t *testing.T) {
	f := newFixture(t)
	principal := "/dav/" + f.household.ID.String() + "/"
	xml := []string{"Content-Type", "text/xml", "Depth", "0"}

	// The root answers for itself, and points at the principal.
	res, body := f.do(t, "PROPFIND", "/dav/", f.password, `<?xml version="1.0"?><A:propfind xmlns:A="DAV:"><A:prop><A:current-user-principal/><A:principal-URL/><A:resourcetype/></A:prop></A:propfind>`, xml...)
	if res.StatusCode != http.StatusMultiStatus || !strings.Contains(body, "<href xmlns=\"DAV:\">/dav/</href><propstat>") ||
		!strings.Contains(body, "<current-user-principal xmlns=\"DAV:\"><href xmlns=\"DAV:\">"+principal+"</href>") {
		t.Fatalf("root: %d %s", res.StatusCode, body)
	}

	// The principal is found with or without its trailing slash, and with no body at all.
	for _, p := range []string{principal, strings.TrimSuffix(principal, "/")} {
		res, body := f.do(t, "PROPFIND", p, f.password, "", xml...)
		if res.StatusCode != http.StatusMultiStatus || !strings.Contains(body, "<calendar-home-set xmlns=\"urn:ietf:params:xml:ns:caldav\"><href xmlns=\"DAV:\">"+f.home+"</href>") ||
			!strings.Contains(body, "<principal-URL") || !strings.Contains(body, "<href xmlns=\"DAV:\">"+principal+"</href><propstat>") {
			t.Fatalf("principal %q: %d %s", p, res.StatusCode, body)
		}
	}
	if res, body := f.do(t, "PROPFIND", "/dav/00000000-0000-0000-0000-000000000000/", f.password, "", xml...); res.StatusCode != http.StatusNotFound {
		t.Fatalf("another principal: %d %s", res.StatusCode, body)
	}

	// The lists carry a ctag that changes when an object in them does.
	homeProps := `<?xml version="1.0"?><A:propfind xmlns:A="DAV:" xmlns:B="urn:ietf:params:xml:ns:caldav" xmlns:C="http://calendarserver.org/ns/"><A:prop><C:getctag/><A:current-user-privilege-set/><A:displayname/><A:resourcetype/><B:supported-calendar-component-set/><A:supported-report-set/></A:prop></A:propfind>`
	ctag := func() string {
		t.Helper()
		res, body := f.do(t, "PROPFIND", strings.TrimSuffix(f.home, "/"), f.password, homeProps, "Content-Type", "text/xml", "Depth", "1")
		if res.StatusCode != http.StatusMultiStatus || strings.Count(body, "<response>") != 5 ||
			!strings.Contains(body, "<write-content/>") || !strings.Contains(body, "<calendar-multiget") {
			t.Fatalf("home: %d %s", res.StatusCode, body)
		}
		_, rest, _ := strings.Cut(body, "<href xmlns=\"DAV:\">"+f.home+"events/</href>")
		_, rest, _ = strings.Cut(rest, `<getctag xmlns="http://calendarserver.org/ns/">`)
		tag, _, _ := strings.Cut(rest, "<")
		if tag == "" {
			t.Fatalf("no ctag: %s", body)
		}
		return tag
	}
	before := ctag()
	event := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:x\r\nBEGIN:VEVENT\r\nUID:E2\r\nDTSTAMP:20261028T100000Z\r\n" +
		"DTSTART:20261105T180000Z\r\nDTEND:20261105T200000Z\r\nSUMMARY:Cinema\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	res, _ = f.do(t, "PUT", f.home+"events/E2.ics", f.password, event, "Content-Type", "text/calendar")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("put: %d", res.StatusCode)
	}
	if ctag() == before {
		t.Fatal("ctag unchanged after a change")
	}

	// A list's members come with the ETag a GET gives.
	res, body = f.do(t, "PROPFIND", f.home+"events", f.password, `<?xml version="1.0"?><A:propfind xmlns:A="DAV:"><A:prop><A:getetag/><A:getcontenttype/></A:prop></A:propfind>`, "Content-Type", "text/xml", "Depth", "1")
	get, _ := f.do(t, "GET", f.home+"events/E2.ics", f.password, "")
	if res.StatusCode != http.StatusMultiStatus || !strings.Contains(body, f.home+"events/E2.ics</href>") ||
		!strings.Contains(body, "<getetag xmlns=\"DAV:\">"+strings.ReplaceAll(get.Header.Get("ETag"), `"`, "&#34;")+"</getetag>") {
		t.Fatalf("list: %d %s (etag %s)", res.StatusCode, body, get.Header.Get("ETag"))
	}
}
