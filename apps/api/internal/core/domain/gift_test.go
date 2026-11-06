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
