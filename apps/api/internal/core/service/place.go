package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// placeSuggestions is how many places one search offers.
const placeSuggestions = 5

// Places suggests towns and cities while someone types one. Without a search (nil) it suggests
// nothing, and people just type the place.
type Places struct{ search port.PlaceSearch }

func NewPlaces(search port.PlaceSearch) *Places { return &Places{search: search} }

func (s *Places) Search(ctx context.Context, _ domain.User, text, lang string) ([]domain.PlaceMatch, error) {
	text = strings.TrimSpace(text)
	if s.search == nil || utf8.RuneCountInString(text) < domain.MinPlaceQuery || utf8.RuneCountInString(text) > domain.MaxDatePlace {
		return []domain.PlaceMatch{}, nil
	}
	return s.search.Search(ctx, text, lang, placeSuggestions)
}
