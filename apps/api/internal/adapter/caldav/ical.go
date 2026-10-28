package caldav

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-ical"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// reminderHour is when a reminder an app gives only a date is due, in the household's time zone.
const reminderHour = 9

var (
	toRule   = map[domain.Repeat]string{domain.RepeatDaily: "DAILY", domain.RepeatWeekly: "WEEKLY", domain.RepeatMonthly: "MONTHLY", domain.RepeatYearly: "YEARLY"}
	fromRule = map[string]domain.Repeat{"DAILY": domain.RepeatDaily, "WEEKLY": domain.RepeatWeekly, "MONTHLY": domain.RepeatMonthly, "YEARLY": domain.RepeatYearly}
)

// toICal writes an event as a VEVENT and a reminder as a VTODO, and returns the encoded object
// too: its ETag is a hash of it. Times carry the household's time zone, so repeats keep their
// wall-clock time.
func toICal(e domain.Entry, loc *time.Location) (*ical.Calendar, []byte, error) {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Homeloom//Homeloom//EN")

	comp := ical.NewComponent(ical.CompEvent)
	if e.Kind == domain.KindReminder {
		comp = ical.NewComponent(ical.CompToDo)
	}
	uid := e.ICalUID
	if uid == "" {
		uid = e.ID.String() + "@homeloom"
	}
	comp.Props.SetText(ical.PropUID, uid)
	comp.Props.SetDateTime(ical.PropDateTimeStamp, e.UpdatedAt.UTC())
	comp.Props.SetDateTime(ical.PropCreated, e.CreatedAt.UTC())
	comp.Props.SetDateTime(ical.PropLastModified, e.UpdatedAt.UTC())
	comp.Props.SetText(ical.PropSummary, e.Text)

	var start *time.Time
	switch {
	case e.Kind == domain.KindEvent && e.AllDay:
		last := *e.StartsAt
		if e.EndsAt != nil {
			last = *e.EndsAt
		}
		// DTEND is exclusive for dates: the day after the last one.
		comp.Props.SetDate(ical.PropDateTimeStart, e.StartsAt.In(loc))
		comp.Props.SetDate(ical.PropDateTimeEnd, last.In(loc).AddDate(0, 0, 1))
		start = e.StartsAt
	case e.Kind == domain.KindEvent:
		comp.Props.SetDateTime(ical.PropDateTimeStart, e.StartsAt.In(loc))
		if e.EndsAt != nil {
			comp.Props.SetDateTime(ical.PropDateTimeEnd, e.EndsAt.In(loc))
		}
		start = e.StartsAt
	case e.DueAt != nil:
		// A repeating to-do repeats from its start, so it gets one at its due time.
		comp.Props.SetDateTime(ical.PropDateTimeStart, e.DueAt.In(loc))
		comp.Props.SetDateTime(ical.PropDue, e.DueAt.In(loc))
		start = e.DueAt
	}
	if rule, ok := toRule[e.Repeat]; ok && start != nil {
		p := ical.NewProp(ical.PropRecurrenceRule)
		p.Value = "FREQ=" + rule
		comp.Props.Set(p)
	}
	if e.Kind == domain.KindReminder {
		status := "NEEDS-ACTION"
		if e.DoneAt != nil {
			status = "COMPLETED"
			comp.Props.SetDateTime(ical.PropCompleted, e.DoneAt.UTC())
		}
		comp.Props.SetText(ical.PropStatus, status)
	}
	if e.Kind == domain.KindEvent && e.AlertMinutes != nil {
		alarm := ical.NewComponent(ical.CompAlarm)
		alarm.Props.SetText(ical.PropAction, "DISPLAY")
		alarm.Props.SetText(ical.PropDescription, e.Text)
		trigger := ical.NewProp(ical.PropTrigger)
		trigger.Value = fmt.Sprintf("-PT%dM", *e.AlertMinutes)
		alarm.Props.Set(trigger)
		comp.Children = append(comp.Children, alarm)
	}
	cal.Children = append(cal.Children, comp)

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, nil, fmt.Errorf("encode entry %s: %w", e.ID, err)
	}
	return cal, buf.Bytes(), nil
}

func etag(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

// master is the object's main component: the one without a RECURRENCE-ID. Changes to single
// times of a repeat are not kept, as Homeloom repeats all times alike.
func master(cal *ical.Calendar, name string) *ical.Component {
	var first *ical.Component
	for _, c := range cal.Children {
		if c.Name != name {
			continue
		}
		if c.Props.Get(ical.PropRecurrenceID) == nil {
			return c
		}
		if first == nil {
			first = c
		}
	}
	return first
}

// fromICal reads what Homeloom keeps of a VEVENT or VTODO. Times without a zone are in loc.
func fromICal(c *ical.Component, kind domain.EntryKind, loc *time.Location) (domain.SyncEntry, error) {
	var in domain.SyncEntry
	if c == nil {
		return in, errors.New("no event or to-do")
	}
	text, _ := c.Props.Text(ical.PropSummary)
	in.Text = truncate(strings.TrimSpace(text), domain.MaxEventText)
	repeat, err := repeatOf(c)
	if err != nil {
		return in, err
	}

	switch kind {
	case domain.KindEvent:
		p := c.Props.Get(ical.PropDateTimeStart)
		if p == nil {
			return in, errors.New("an event needs DTSTART")
		}
		start, allDay, err := when(p, loc)
		if err != nil {
			return in, err
		}
		end, err := endOf(c, start, allDay, loc)
		if err != nil {
			return in, err
		}
		in.StartsAt, in.AllDay, in.Repeat = &start, allDay, repeat
		switch {
		case allDay:
			// The last day is the one before the exclusive end.
			if last := end.AddDate(0, 0, -1); last.After(start) {
				in.EndsAt = &last
			}
		case end.After(start):
			in.EndsAt = &end
		}
		in.AlertMinutes = alertOf(c, start)
	case domain.KindReminder:
		if p := c.Props.Get(ical.PropDue); p != nil {
			due, dateOnly, err := when(p, loc)
			if err != nil {
				return in, err
			}
			if dateOnly {
				due = due.Add(reminderHour * time.Hour)
			}
			in.DueAt = &due
			in.Repeat = repeat
		}
		status, _ := c.Props.Text(ical.PropStatus)
		in.Done = strings.EqualFold(status, "COMPLETED") || c.Props.Get(ical.PropCompleted) != nil && status == ""
	}
	return in, nil
}

// when reads a date or date-time, and says whether it is a date. A TZID Go doesn't know is read in
// loc.
func when(p *ical.Prop, loc *time.Location) (time.Time, bool, error) {
	dateOnly := p.ValueType() == ical.ValueDate || p.ValueType() == ical.ValueDefault && len(p.Value) == len("20060102")
	if dateOnly {
		t, err := time.ParseInLocation("20060102", p.Value, loc)
		return t, true, err
	}
	if tzid := p.Params.Get(ical.PropTimezoneID); tzid != "" {
		if _, err := time.LoadLocation(tzid); err != nil {
			t, err := time.ParseInLocation("20060102T150405", p.Value, loc)
			return t, false, err
		}
	}
	t, err := p.DateTime(loc)
	return t, false, err
}

// endOf is the exclusive end of an event, from DTEND or DURATION.
func endOf(c *ical.Component, start time.Time, allDay bool, loc *time.Location) (time.Time, error) {
	if p := c.Props.Get(ical.PropDateTimeEnd); p != nil {
		end, _, err := when(p, loc)
		return end, err
	}
	if p := c.Props.Get(ical.PropDuration); p != nil {
		d, err := p.Duration()
		if err != nil {
			return time.Time{}, err
		}
		if allDay {
			return start.AddDate(0, 0, int(d/(24*time.Hour))), nil
		}
		return start.Add(d), nil
	}
	if allDay {
		return start.AddDate(0, 0, 1), nil
	}
	return start, nil
}

// alertOf is the first alarm before or at the start, in minutes before it.
func alertOf(c *ical.Component, start time.Time) *int {
	for _, a := range c.Children {
		if a.Name != ical.CompAlarm {
			continue
		}
		p := a.Props.Get(ical.PropTrigger)
		if p == nil {
			continue
		}
		var before time.Duration
		if p.ValueType() == ical.ValueDateTime {
			at, err := p.DateTime(time.UTC)
			if err != nil {
				continue
			}
			before = start.Sub(at)
		} else {
			if strings.EqualFold(p.Params.Get("RELATED"), "END") {
				continue
			}
			d, err := p.Duration()
			if err != nil {
				continue
			}
			before = -d
		}
		if before < 0 || before > domain.MaxAlertMinutes*time.Minute {
			continue
		}
		m := int(before / time.Minute)
		return &m
	}
	return nil
}

// repeatOf reads the frequency of an RRULE. Homeloom repeats every day, week, month or year, so
// intervals, weekdays and ends are not kept.
func repeatOf(c *ical.Component) (domain.Repeat, error) {
	p := c.Props.Get(ical.PropRecurrenceRule)
	if p == nil {
		return domain.RepeatNever, nil
	}
	for part := range strings.SplitSeq(p.Value, ";") {
		if freq, ok := strings.CutPrefix(strings.ToUpper(part), "FREQ="); ok {
			if r, ok := fromRule[freq]; ok {
				return r, nil
			}
			return "", fmt.Errorf("repeats %s are not supported", strings.ToLower(freq))
		}
	}
	return "", errors.New("RRULE without FREQ")
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
