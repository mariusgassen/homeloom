package domain

import (
	"testing"
	"time"
)

func TestBirthdayYear(t *testing.T) {
	now := time.Now().Year()
	for _, tc := range []struct {
		b    Birthday
		want bool
	}{
		{Birthday{Month: time.February, Day: 29}, true},
		{Birthday{Month: time.May, Day: 4, Year: 1950}, true},
		{Birthday{Month: time.May, Day: 4, Year: now}, true},
		{Birthday{Month: time.February, Day: 29, Year: 1996}, true},
		{Birthday{Month: time.February, Day: 29, Year: 1999}, false},
		{Birthday{Month: time.May, Day: 4, Year: 1899}, false},
		{Birthday{Month: time.May, Day: 4, Year: now + 1}, false},
	} {
		if got := tc.b.Valid(); got != tc.want {
			t.Errorf("%+v valid = %v, want %v", tc.b, got, tc.want)
		}
	}
	if got := (Birthday{Month: time.May, Day: 4, Year: 1990}).Age(time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)); got != 36 {
		t.Errorf("age = %d, want 36", got)
	}
	if got := (Birthday{Month: time.May, Day: 4}).Age(time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)); got != 0 {
		t.Errorf("age without year = %d, want 0", got)
	}
}

func TestOccasionNext(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	today := day(2026, time.December, 18) // a Friday
	for name, tc := range map[string]struct {
		o    Occasion
		next time.Time
		days int
		ok   bool
	}{
		"yearly":                  {Occasion{On: day(2020, 12, 25), Repeat: RepeatYearly}, day(2026, 12, 25), 7, true},
		"yearly, passed":          {Occasion{On: day(2020, 6, 3), Repeat: RepeatYearly}, day(2027, 6, 3), 167, true},
		"yearly, starts later":    {Occasion{On: day(2028, 6, 3), Repeat: RepeatYearly}, day(2028, 6, 3), 533, true},
		"weekly on a Monday":      {Occasion{On: day(2026, 1, 5), Repeat: RepeatWeekly}, day(2026, 12, 21), 3, true},
		"weekly, today":           {Occasion{On: day(2026, 1, 2), Repeat: RepeatWeekly}, today, 0, true},
		"monthly on the 31st":     {Occasion{On: day(2026, 1, 31), Repeat: RepeatMonthly}, day(2026, 12, 31), 13, true},
		"monthly, next month":     {Occasion{On: day(2026, 1, 10), Repeat: RepeatMonthly}, day(2027, 1, 10), 23, true},
		"monthly, short February": {Occasion{On: day(2026, 1, 31), Repeat: RepeatMonthly}, day(2027, 2, 28), 72, true},
		"once, to come":           {Occasion{On: day(2026, 12, 20), Repeat: RepeatNever}, day(2026, 12, 20), 2, true},
		"once, passed":            {Occasion{On: day(2026, 12, 17), Repeat: RepeatNever}, time.Time{}, 0, false},
	} {
		t.Run(name, func(t *testing.T) {
			now := today
			if name == "monthly, short February" {
				now = day(2027, time.January, 31).AddDate(0, 0, 1) // 1 February 2027
				tc.days = 27
			}
			next, days, ok := tc.o.Next(now)
			if ok != tc.ok || !next.Equal(tc.next) || (ok && days != tc.days) {
				t.Fatalf("Next = %v, %d, %v; want %v, %d, %v", next, days, ok, tc.next, tc.days, tc.ok)
			}
		})
	}
}
