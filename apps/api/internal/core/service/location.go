package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Location keeps where a member's device is, so their weather is from where they are.
type Location struct {
	repo  port.LocationRepository
	namer port.PlaceNamer // nil: positions are kept without a name
}

func NewLocation(repo port.LocationRepository) *Location { return &Location{repo: repo} }

// WithNamer names the town a position is in.
func (s *Location) WithNamer(n port.PlaceNamer) *Location {
	s.namer = n
	return s
}

func (s *Location) Set(ctx context.Context, actor domain.User, at domain.Coords, lang string) error {
	if !at.Valid() {
		return fmt.Errorf("%w: no such position", domain.ErrInvalid)
	}
	// A town's worth of precision is all a forecast needs, and all that is kept.
	l := domain.MemberLocation{Coords: roundCoords(at)}
	if s.namer != nil {
		name, err := s.namer.PlaceName(ctx, l.Coords, lang)
		if err != nil {
			// A name that can't be found doesn't stop the weather.
			slog.WarnContext(ctx, "location: place name", "err", err)
		}
		l.Place = name
	}
	return s.repo.SaveLocation(ctx, actor.ID, l)
}

func (s *Location) Clear(ctx context.Context, actor domain.User) error {
	return s.repo.DeleteLocation(ctx, actor.ID)
}

func roundCoords(c domain.Coords) domain.Coords {
	return domain.Coords{Latitude: math.Round(c.Latitude*100) / 100, Longitude: math.Round(c.Longitude*100) / 100}
}
