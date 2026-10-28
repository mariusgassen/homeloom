package http

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// icalendar writes entries as an RFC 5545 calendar for subscription. Events keep their times;
// reminders become moments at their due time. Alerts are left out: Homeloom notifies itself.
func icalendar(es []domain.Entry, loc *time.Location, now time.Time) []byte {
	var b icalWriter
	b.line("BEGIN:VCALENDAR")
	b.line("VERSION:2.0")
	b.line("PRODID:-//Homeloom//Homeloom//EN")
	b.line("CALSCALE:GREGORIAN")
	b.line("METHOD:PUBLISH")
	b.line("X-WR-CALNAME:Homeloom")
	b.line("X-WR-TIMEZONE:" + loc.String())
	b.line("REFRESH-INTERVAL;VALUE=DURATION:PT15M")
	b.line("X-PUBLISHED-TTL:PT15M")
	for _, e := range es {
		b.line("BEGIN:VEVENT")
		b.line("UID:" + e.ID.String() + "@homeloom")
		b.line("DTSTAMP:" + now.UTC().Format("20060102T150405Z"))
		b.line("LAST-MODIFIED:" + e.UpdatedAt.UTC().Format("20060102T150405Z"))
		b.line("SUMMARY:" + icalText(e.Text))
		switch {
		case e.Kind == domain.KindEvent && e.AllDay:
			last := *e.StartsAt
			if e.EndsAt != nil {
				last = *e.EndsAt
			}
			// DTEND is exclusive for dates: the day after the last one.
			b.line("DTSTART;VALUE=DATE:" + e.StartsAt.In(loc).Format("20060102"))
			b.line("DTEND;VALUE=DATE:" + last.In(loc).AddDate(0, 0, 1).Format("20060102"))
		case e.Kind == domain.KindEvent:
			b.line("DTSTART" + icalTime(*e.StartsAt, loc))
			if e.EndsAt != nil {
				b.line("DTEND" + icalTime(*e.EndsAt, loc))
			}
		default:
			b.line("DTSTART" + icalTime(*e.DueAt, loc))
		}
		if rule, ok := icalRepeat[e.Repeat]; ok {
			b.line("RRULE:FREQ=" + rule)
		}
		if e.Visibility == domain.VisibilityPrivate {
			b.line("CLASS:PRIVATE")
		}
		b.line("END:VEVENT")
	}
	b.line("END:VCALENDAR")
	return b.Bytes()
}

var icalRepeat = map[domain.Repeat]string{
	domain.RepeatDaily: "DAILY", domain.RepeatWeekly: "WEEKLY", domain.RepeatMonthly: "MONTHLY", domain.RepeatYearly: "YEARLY",
}

// icalTime is a property's value with its time zone, so repeats keep their local time across
// daylight saving time.
func icalTime(t time.Time, loc *time.Location) string {
	if loc == time.UTC {
		return ":" + t.UTC().Format("20060102T150405Z")
	}
	return ";TZID=" + loc.String() + ":" + t.In(loc).Format("20060102T150405")
}

func icalText(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`).Replace(s)
}

type icalWriter struct{ bytes.Buffer }

// line writes one content line, folded at 75 octets without splitting a character.
func (w *icalWriter) line(s string) {
	limit := 75
	for len(s) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		fmt.Fprintf(w, "%s\r\n ", s[:cut])
		s = s[cut:]
		limit = 74 // the leading space counts
	}
	fmt.Fprintf(w, "%s\r\n", s)
}
