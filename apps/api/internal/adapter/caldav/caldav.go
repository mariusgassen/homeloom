// Package caldav is the driving CalDAV adapter: it serves each member's events and reminders at
// /dav, so calendar apps such as iOS Calendar and Reminders sync them both ways. It signs in with
// a personal calendar password (any user name), which acts as its owner in one household.
package caldav

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

// Prefix is where CalDAV is served. WellKnown redirects there, so an app finds it from the
// server's name alone.
const (
	Prefix    = "/dav"
	WellKnown = "/.well-known/caldav"
	// maxBody bounds one calendar object an app sends.
	maxBody = 256 << 10
)

type Deps struct {
	Agents  port.AgentAccessService
	Entries port.EntrySyncService
	// Location is the household's time zone, for times an app sends without one.
	Location *time.Location
}

// list is one calendar collection. Paths are /dav/<household>/calendars/<slug>/<name>.
type list struct {
	slug string
	domain.SyncList
	comp, title string
}

var lists = []list{
	{"events", domain.SyncList{Kind: domain.KindEvent, Visibility: domain.VisibilityShared}, ical.CompEvent, "dav.shared"},
	{"events-private", domain.SyncList{Kind: domain.KindEvent, Visibility: domain.VisibilityPrivate}, ical.CompEvent, "dav.private"},
	{"reminders", domain.SyncList{Kind: domain.KindReminder, Visibility: domain.VisibilityShared}, ical.CompToDo, "dav.shared"},
	{"reminders-private", domain.SyncList{Kind: domain.KindReminder, Visibility: domain.VisibilityPrivate}, ical.CompToDo, "dav.private"},
}

type grantKey struct{}

// NewHandler serves CalDAV at Prefix and the redirect at WellKnown.
func NewHandler(d Deps) http.Handler {
	h := &caldav.Handler{Backend: &backend{d}, Prefix: Prefix}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == WellKnown {
			http.Redirect(w, r, Prefix+"/", http.StatusMovedPermanently)
			return
		}
		// The password is a personal token; the user name means nothing.
		_, token, _ := r.BasicAuth()
		g, err := d.Agents.AuthenticateCalendar(r.Context(), token)
		switch {
		case errors.Is(err, domain.ErrUnauthenticated):
			w.Header().Set("WWW-Authenticate", `Basic realm="Homeloom", charset="UTF-8"`)
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		case err != nil:
			slog.ErrorContext(r.Context(), "caldav authentication unavailable", "err", err)
			http.Error(w, "authentication unavailable", http.StatusServiceUnavailable)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), grantKey{}, g))
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		if r.Method == "PROPPATCH" {
			proppatch(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// proppatch accepts an app's own settings for a list, such as its colour or order, without
// keeping them: Homeloom decides what its lists are. Apps show an error when it is refused.
func proppatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Set []struct {
			Prop struct {
				Any []struct{ XMLName xml.Name } `xml:",any"`
			} `xml:"prop"`
		} `xml:"set"`
	}
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid propertyupdate", http.StatusBadRequest)
		return
	}
	var props strings.Builder
	for _, s := range req.Set {
		for _, p := range s.Prop.Any {
			fmt.Fprintf(&props, `<%s xmlns="%s"/>`, p.XMLName.Local, xmlEscape(p.XMLName.Space))
		}
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><multistatus xmlns="DAV:"><response><href>%s</href><propstat><prop>%s</prop><status>HTTP/1.1 200 OK</status></propstat></response></multistatus>`,
		xmlEscape(r.URL.Path), props.String())
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

type backend struct{ Deps }

func grant(ctx context.Context) domain.AgentGrant {
	g, _ := ctx.Value(grantKey{}).(domain.AgentGrant)
	return g
}

func (b *backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return Prefix + "/" + grant(ctx).HouseholdID.String() + "/", nil
}

func (b *backend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	p, _ := b.CurrentUserPrincipal(ctx)
	return p + "calendars/", nil
}

func (b *backend) CreateCalendar(context.Context, *caldav.Calendar) error {
	return webdav.NewHTTPError(http.StatusForbidden, errors.New("the lists are fixed"))
}

func (b *backend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	out := make([]caldav.Calendar, len(lists))
	for i, l := range lists {
		out[i] = b.calendar(ctx, l)
	}
	return out, nil
}

func (b *backend) GetCalendar(ctx context.Context, path string) (*caldav.Calendar, error) {
	l, name, err := b.parse(ctx, path)
	if err != nil {
		return nil, err
	}
	if name != "" {
		return nil, webdav.NewHTTPError(http.StatusNotFound, nil)
	}
	c := b.calendar(ctx, l)
	return &c, nil
}

func (b *backend) calendar(ctx context.Context, l list) caldav.Calendar {
	home, _ := b.CalendarHomeSetPath(ctx)
	return caldav.Calendar{
		Path:                  home + l.slug + "/",
		Name:                  i18n.T(i18n.FromContext(ctx), l.title),
		SupportedComponentSet: []string{l.comp},
		MaxResourceSize:       maxBody,
	}
}

func (b *backend) GetCalendarObject(ctx context.Context, path string, _ *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	l, name, err := b.parse(ctx, path)
	if err != nil {
		return nil, err
	}
	g := grant(ctx)
	e, err := b.Entries.SyncEntry(ctx, g.User, g.HouseholdID, l.SyncList, name)
	if err != nil {
		return nil, httpError(ctx, err)
	}
	return b.object(ctx, l, e)
}

func (b *backend) ListCalendarObjects(ctx context.Context, path string, _ *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	l, name, err := b.parse(ctx, path)
	if err != nil {
		return nil, err
	}
	if name != "" {
		return nil, webdav.NewHTTPError(http.StatusNotFound, nil)
	}
	g := grant(ctx)
	es, err := b.Entries.SyncEntries(ctx, g.User, g.HouseholdID, l.SyncList)
	if err != nil {
		return nil, httpError(ctx, err)
	}
	out := make([]caldav.CalendarObject, 0, len(es))
	for _, e := range es {
		o, err := b.object(ctx, l, e)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, nil
}

// QueryCalendarObjects answers with every object of the asked-for kind: a list is small, and an
// app takes what it asked for from it. Time ranges are not applied, so repeats are never missed.
func (b *backend) QueryCalendarObjects(ctx context.Context, path string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	l, _, err := b.parse(ctx, path)
	if err != nil {
		return nil, err
	}
	if f := query.CompFilter; f.Name != "" && f.Name != ical.CompCalendar ||
		len(f.Comps) > 0 && f.Comps[0].Name != l.comp {
		return nil, nil
	}
	return b.ListCalendarObjects(ctx, path, nil)
}

func (b *backend) PutCalendarObject(ctx context.Context, path string, cal *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	l, name, err := b.parse(ctx, path)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, webdav.NewHTTPError(http.StatusMethodNotAllowed, nil)
	}
	kind, uid, err := caldav.ValidateCalendarObject(cal)
	if err != nil {
		return nil, caldav.NewPreconditionError(caldav.PreconditionValidCalendarObjectResource)
	}
	if kind != l.comp {
		return nil, caldav.NewPreconditionError(caldav.PreconditionSupportedCalendarComponent)
	}
	in, err := fromICal(master(cal, kind), l.Kind, b.Location)
	if err != nil {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	in.UID = uid

	g := grant(ctx)
	if opts.IfMatch.IsSet() || opts.IfNoneMatch.IsSet() {
		var etag string
		e, err := b.Entries.SyncEntry(ctx, g.User, g.HouseholdID, l.SyncList, name)
		switch {
		case err == nil:
			o, err := b.object(ctx, l, e)
			if err != nil {
				return nil, err
			}
			etag = o.ETag
		case !errors.Is(err, domain.ErrNotFound):
			return nil, httpError(ctx, err)
		}
		if err := conditions(etag, opts); err != nil {
			return nil, err
		}
	}

	e, _, err := b.Entries.SyncPut(ctx, g.User, g.HouseholdID, l.SyncList, name, in)
	if err != nil {
		return nil, httpError(ctx, err)
	}
	return b.object(ctx, l, e)
}

// conditions checks If-Match and If-None-Match against the current ETag ("" when there is none).
func conditions(etag string, opts *caldav.PutCalendarObjectOptions) error {
	failed := webdav.NewHTTPError(http.StatusPreconditionFailed, nil)
	if opts.IfNoneMatch.IsSet() && etag != "" {
		if ok, err := opts.IfNoneMatch.MatchETag(etag); err != nil || ok {
			return failed
		}
	}
	if opts.IfMatch.IsSet() {
		if etag == "" {
			return failed
		}
		if ok, err := opts.IfMatch.MatchETag(etag); err != nil || !ok {
			return failed
		}
	}
	return nil
}

func (b *backend) DeleteCalendarObject(ctx context.Context, path string) error {
	l, name, err := b.parse(ctx, path)
	if err != nil {
		return err
	}
	if name == "" {
		return webdav.NewHTTPError(http.StatusForbidden, errors.New("the lists are fixed"))
	}
	g := grant(ctx)
	return httpError(ctx, b.Entries.SyncDelete(ctx, g.User, g.HouseholdID, l.SyncList, name))
}

func (b *backend) object(ctx context.Context, l list, e domain.Entry) (*caldav.CalendarObject, error) {
	home, _ := b.CalendarHomeSetPath(ctx)
	cal, data, err := toICal(e, b.Location)
	if err != nil {
		return nil, err
	}
	return &caldav.CalendarObject{
		Path:          home + l.slug + "/" + e.SyncName(),
		ModTime:       e.UpdatedAt,
		ContentLength: int64(len(data)),
		ETag:          etag(data),
		Data:          cal,
	}, nil
}

// parse splits a path below the caller's calendar home into its list and object name ("" for the
// list itself). Another household's paths are not found.
func (b *backend) parse(ctx context.Context, path string) (list, string, error) {
	home, _ := b.CalendarHomeSetPath(ctx)
	rest, ok := strings.CutPrefix(path, home)
	if !ok {
		if strings.TrimSuffix(path, "/")+"/" == home {
			return list{}, "", webdav.NewHTTPError(http.StatusMethodNotAllowed, nil)
		}
		return list{}, "", webdav.NewHTTPError(http.StatusNotFound, nil)
	}
	slug, name, _ := strings.Cut(rest, "/")
	for _, l := range lists {
		if l.slug == slug && !strings.Contains(name, "/") {
			return l, name, nil
		}
	}
	return list{}, "", webdav.NewHTTPError(http.StatusNotFound, nil)
}

// httpError turns a core error into the status an app understands; others are logged, not shown.
func httpError(ctx context.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return webdav.NewHTTPError(http.StatusNotFound, nil)
	case errors.Is(err, domain.ErrForbidden):
		return webdav.NewHTTPError(http.StatusForbidden, err)
	case errors.Is(err, domain.ErrConflict):
		return webdav.NewHTTPError(http.StatusConflict, err)
	case errors.Is(err, domain.ErrInvalid):
		return webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	slog.ErrorContext(ctx, "caldav request failed", "err", err)
	return webdav.NewHTTPError(http.StatusInternalServerError, errors.New("internal error"))
}
