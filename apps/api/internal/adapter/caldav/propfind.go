package caldav

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
)

// The collections (the root, the principal, the calendar home and the lists) are answered here
// rather than by go-webdav, whose answers iOS does not accept: it answers the root with the
// principal's href, finds nothing at a collection's path without its trailing slash, refuses a
// PROPFIND without a body, and has no getctag, privileges on the principal or supported reports.
// Calendar objects are still go-webdav's.

const (
	nsDAV = "DAV:"
	nsCal = "urn:ietf:params:xml:ns:caldav"
	nsCS  = "http://calendarserver.org/ns/"
)

func davName(local string) xml.Name { return xml.Name{Space: nsDAV, Local: local} }
func calName(local string) xml.Name { return xml.Name{Space: nsCal, Local: local} }

// propfindReq is a PROPFIND body. No body, or an empty one, asks for all properties.
type propfindReq struct {
	AllProp *struct{} `xml:"DAV: allprop"`
	Prop    *struct {
		Any []struct{ XMLName xml.Name } `xml:",any"`
	} `xml:"DAV: prop"`
}

func (q propfindReq) all() bool { return q.Prop == nil }

// props maps a property to its inner XML; each is only made when it is asked for.
type props map[xml.Name]func() (string, error)

func value(inner string) func() (string, error) { return func() (string, error) { return inner, nil } }

// response is one multistatus response: what was asked for and is there, then what is not.
func (q propfindReq) response(href string, p props) (string, error) {
	var names []xml.Name
	if q.all() {
		for n := range p {
			// calendar-data is only sent when asked for by name.
			if n != calName("calendar-data") {
				names = append(names, n)
			}
		}
		slices.SortFunc(names, func(a, b xml.Name) int { return strings.Compare(a.Space+a.Local, b.Space+b.Local) })
	} else {
		for _, a := range q.Prop.Any {
			names = append(names, a.XMLName)
		}
	}
	var found, missing strings.Builder
	for _, n := range names {
		f, ok := p[n]
		if !ok {
			missing.WriteString(element(n, ""))
			continue
		}
		inner, err := f()
		if err != nil {
			return "", err
		}
		found.WriteString(element(n, inner))
	}
	var b strings.Builder
	b.WriteString("<response>" + hrefXML(href))
	if found.Len() > 0 {
		b.WriteString("<propstat><prop>" + found.String() + "</prop><status>HTTP/1.1 200 OK</status></propstat>")
	}
	if missing.Len() > 0 {
		b.WriteString("<propstat><prop>" + missing.String() + "</prop><status>HTTP/1.1 404 Not Found</status></propstat>")
	}
	b.WriteString("</response>")
	return b.String(), nil
}

func element(n xml.Name, inner string) string {
	return "<" + n.Local + ` xmlns="` + xmlEscape(n.Space) + `">` + inner + "</" + n.Local + ">"
}

func hrefXML(p string) string {
	return `<href xmlns="DAV:">` + xmlEscape((&url.URL{Path: p}).String()) + "</href>"
}

func privileges(names ...string) string {
	var b strings.Builder
	for _, n := range names {
		b.WriteString(`<privilege xmlns="DAV:"><` + n + "/></privilege>")
	}
	return b.String()
}

// propfind answers a PROPFIND on a collection and reports true; for anything else, it reports
// false and leaves the request to go-webdav.
func (b *backend) propfind(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	principal, _ := b.CurrentUserPrincipal(ctx)
	home, _ := b.CalendarHomeSetPath(ctx)
	p := path.Clean(r.URL.Path) + "/"
	var cal *list
	switch {
	case p == Prefix+"/", p == principal, p == home:
	case strings.HasPrefix(p, home):
		rest := strings.TrimSuffix(strings.TrimPrefix(p, home), "/")
		if strings.Contains(rest, "/") {
			// An object: go-webdav finds it, or answers that it is not there.
			return false
		}
		for i := range lists {
			if lists[i].slug == rest {
				cal = &lists[i]
			}
		}
		if cal == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return true
		}
	default:
		http.Error(w, "not found", http.StatusNotFound)
		return true
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid propfind", http.StatusBadRequest)
		return true
	}
	var q propfindReq
	if len(bytes.TrimSpace(body)) > 0 {
		if err := xml.Unmarshal(body, &q); err != nil {
			http.Error(w, "invalid propfind", http.StatusBadRequest)
			return true
		}
	}
	// Depth infinity is taken as 1: nothing here is deeper than its children.
	deep := r.Header.Get("Depth") != "0"

	var out []string
	add := func(href string, p props) error {
		resp, err := q.response(href, p)
		out = append(out, resp)
		return err
	}
	switch p {
	case Prefix + "/":
		err = add(p, b.rootProps(ctx))
		if err == nil && deep {
			err = add(principal, b.principalProps(ctx))
		}
	case principal:
		err = add(p, b.principalProps(ctx))
		if err == nil && deep {
			err = add(home, b.homeProps(ctx))
		}
	case home:
		err = add(p, b.homeProps(ctx))
		for _, l := range lists {
			if err == nil && deep {
				err = add(home+l.slug+"/", b.listProps(ctx, l))
			}
		}
	default:
		err = add(p, b.listProps(ctx, *cal))
		if err == nil && deep {
			var objs []caldav.CalendarObject
			objs, err = b.ListCalendarObjects(ctx, p, nil)
			for _, o := range objs {
				if err == nil {
					err = add(o.Path, objectProps(o))
				}
			}
		}
	}
	if err != nil {
		// httpError has logged what went wrong.
		http.Error(w, "propfind failed", http.StatusInternalServerError)
		return true
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><multistatus xmlns="DAV:">`+strings.Join(out, "")+"</multistatus>")
	return true
}

// common are the properties every collection has.
func (b *backend) common(ctx context.Context, resourcetype string, privs ...string) props {
	principal, _ := b.CurrentUserPrincipal(ctx)
	return props{
		davName("resourcetype"):               value(resourcetype),
		davName("current-user-principal"):     value(hrefXML(principal)),
		davName("owner"):                      value(hrefXML(principal)),
		davName("current-user-privilege-set"): value(privileges(append(privs, "read-current-user-privilege-set")...)),
	}
}

const collection = `<collection xmlns="DAV:"/>`

func (b *backend) rootProps(ctx context.Context) props {
	return b.common(ctx, collection, "read")
}

func (b *backend) principalProps(ctx context.Context) props {
	principal, _ := b.CurrentUserPrincipal(ctx)
	home, _ := b.CalendarHomeSetPath(ctx)
	u := grant(ctx).User
	p := b.common(ctx, collection+`<principal xmlns="DAV:"/>`, "read")
	p[davName("principal-URL")] = value(hrefXML(principal))
	p[davName("principal-collection-set")] = value(hrefXML(Prefix + "/"))
	p[calName("calendar-home-set")] = value(hrefXML(home))
	addresses := hrefXML(principal)
	if u.Email != "" {
		addresses = `<href xmlns="DAV:">mailto:` + xmlEscape(u.Email) + "</href>" + addresses
	}
	p[calName("calendar-user-address-set")] = value(addresses)
	if u.Name != "" {
		p[davName("displayname")] = value(xmlEscape(u.Name))
	}
	return p
}

// homeProps: the lists are fixed, so the home can be read but not added to.
func (b *backend) homeProps(ctx context.Context) props {
	return b.common(ctx, collection, "read")
}

func (b *backend) listProps(ctx context.Context, l list) props {
	c := b.calendar(ctx, l)
	p := b.common(ctx, collection+`<calendar xmlns="urn:ietf:params:xml:ns:caldav"/>`,
		"read", "write", "write-content", "write-properties", "bind", "unbind")
	p[davName("displayname")] = value(xmlEscape(c.Name))
	p[calName("supported-calendar-component-set")] = value(`<comp xmlns="urn:ietf:params:xml:ns:caldav" name="` + l.comp + `"/>`)
	p[calName("supported-calendar-data")] = value(`<calendar-data xmlns="urn:ietf:params:xml:ns:caldav" content-type="text/calendar" version="2.0"/>`)
	p[calName("max-resource-size")] = value(strconv.FormatInt(c.MaxResourceSize, 10))
	p[davName("supported-report-set")] = value(
		`<supported-report xmlns="DAV:"><report><calendar-multiget xmlns="urn:ietf:params:xml:ns:caldav"/></report></supported-report>` +
			`<supported-report xmlns="DAV:"><report><calendar-query xmlns="urn:ietf:params:xml:ns:caldav"/></report></supported-report>`)
	// The ctag changes whenever any object in the list does, so an app only fetches a list
	// that has changed.
	p[xml.Name{Space: nsCS, Local: "getctag"}] = func() (string, error) {
		objs, err := b.ListCalendarObjects(ctx, c.Path, nil)
		if err != nil {
			return "", err
		}
		tags := make([]string, len(objs))
		for i, o := range objs {
			tags[i] = o.Path + " " + o.ETag
		}
		slices.Sort(tags)
		sum := sha256.Sum256([]byte(strings.Join(tags, "\n")))
		return hex.EncodeToString(sum[:16]), nil
	}
	return p
}

func objectProps(o caldav.CalendarObject) props {
	p := props{
		davName("resourcetype"):   value(""),
		davName("getetag"):        value(xmlEscape(strconv.Quote(o.ETag))),
		davName("getcontenttype"): value(ical.MIMEType + "; charset=utf-8"),
		calName("calendar-data"): func() (string, error) {
			var buf bytes.Buffer
			if err := ical.NewEncoder(&buf).Encode(o.Data); err != nil {
				return "", err
			}
			return xmlEscape(buf.String()), nil
		},
	}
	if o.ContentLength > 0 {
		p[davName("getcontentlength")] = value(strconv.FormatInt(o.ContentLength, 10))
	}
	if !o.ModTime.IsZero() {
		p[davName("getlastmodified")] = value(o.ModTime.UTC().Format(http.TimeFormat))
	}
	return p
}
