package http

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func TestICalendar(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 24, 8, 0, 0, 0, berlin)
	long := strings.Repeat("ä", 60) + "; a\nb"
	ics := string(icalendar([]domain.Entry{
		{ID: uuid.New(), Kind: domain.KindEvent, EntryFields: domain.EntryFields{Text: long, StartsAt: &start, Repeat: domain.RepeatDaily, Visibility: domain.VisibilityShared}},
		{ID: uuid.New(), Kind: domain.KindReminder, EntryFields: domain.EntryFields{Text: "Bins", DueAt: &start, Visibility: domain.VisibilityShared}},
	}, berlin, start))

	for _, want := range []string{"DTSTART;TZID=Europe/Berlin:20261024T080000\r\n", "RRULE:FREQ=DAILY\r\n", "SUMMARY:Bins\r\n", "X-WR-TIMEZONE:Europe/Berlin\r\n"} {
		if !strings.Contains(ics, want) {
			t.Fatalf("missing %q:\n%s", want, ics)
		}
	}
	unfolded := strings.ReplaceAll(ics, "\r\n ", "")
	if !strings.Contains(unfolded, "SUMMARY:"+strings.Repeat("ä", 60)+`\; a\nb`+"\r\n") {
		t.Fatalf("summary is escaped and folds back:\n%s", ics)
	}
	for line := range strings.SplitSeq(ics, "\r\n") {
		if len(line) > 75 {
			t.Fatalf("line longer than 75 octets: %q", line)
		}
		if !utf8.ValidString(line) {
			t.Fatalf("folding split a character: %q", line)
		}
	}
}
