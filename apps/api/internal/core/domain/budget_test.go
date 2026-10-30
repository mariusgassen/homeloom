package domain_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func TestSplitEvenlyGivesOddCentsToTheFirst(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	got := domain.SplitEvenly(1001, []uuid.UUID{a, b, c})
	want := []int64{334, 334, 333}
	var sum int64
	for i, sh := range got {
		if sh.Cents != want[i] {
			t.Fatalf("share %d: %d, want %d", i, sh.Cents, want[i])
		}
		sum += sh.Cents
	}
	if sum != 1001 {
		t.Fatalf("shares add up to %d", sum)
	}
}

func TestSettlePaysTheLargestDebtsFirst(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	if got := domain.Settle([]domain.Net{{UserID: a, Cents: 0}, {UserID: b, Cents: 0}}); len(got) != 0 {
		t.Fatalf("even: %v", got)
	}
	got := domain.Settle([]domain.Net{{UserID: a, Cents: 2500}, {UserID: b, Cents: -2500}})
	if len(got) != 1 || got[0] != (domain.Transfer{From: b, To: a, Cents: 2500}) {
		t.Fatalf("two people: %v", got)
	}
	got = domain.Settle([]domain.Net{{UserID: a, Cents: 3000}, {UserID: b, Cents: -1000}, {UserID: c, Cents: -2000}})
	if len(got) != 2 || got[0] != (domain.Transfer{From: c, To: a, Cents: 2000}) || got[1] != (domain.Transfer{From: b, To: a, Cents: 1000}) {
		t.Fatalf("three people: %v", got)
	}
}
