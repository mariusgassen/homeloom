package service_test

import (
	"context"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

type fakeSearch struct{ calls int }

func (f *fakeSearch) Search(context.Context, string, string, int) ([]domain.PlaceMatch, error) {
	f.calls++
	return []domain.PlaceMatch{{Label: "Berlin, Germany"}}, nil
}

func TestPlaces(t *testing.T) {
	ctx := context.Background()
	search := &fakeSearch{}
	svc := service.NewPlaces(search)
	if got, err := svc.Search(ctx, domain.User{}, " B ", "en"); err != nil || len(got) != 0 || search.calls != 0 {
		t.Errorf("too short should ask nothing: %+v, %v, %d calls", got, err, search.calls)
	}
	if got, err := svc.Search(ctx, domain.User{}, "Berl", "en"); err != nil || len(got) != 1 || search.calls != 1 {
		t.Errorf("search: %+v, %v, %d calls", got, err, search.calls)
	}
	if got, err := service.NewPlaces(nil).Search(ctx, domain.User{}, "Berlin", "en"); err != nil || got == nil || len(got) != 0 {
		t.Errorf("without a search: %+v, %v", got, err)
	}
}
