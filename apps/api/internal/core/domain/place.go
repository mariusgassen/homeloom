package domain

import "time"

// MinPlaceQuery is the shortest text a place search starts from.
const MinPlaceQuery = 2

// PlaceMatch is a town or city a search found. Label is how to write it ("Berlin, Germany"): the
// weather lookup finds the same place again from it.
type PlaceMatch struct {
	Label string
}

// Coords is a position on Earth, in degrees.
type Coords struct {
	Latitude, Longitude float64
}

// Valid reports whether the position exists.
func (c Coords) Valid() bool {
	return c.Latitude >= -90 && c.Latitude <= 90 && c.Longitude >= -180 && c.Longitude <= 180
}

// LocationFreshFor is how long a member's shared position counts; after that their weather is the
// household's place again, so a phone that stays closed doesn't forecast a place long left.
const LocationFreshFor = 72 * time.Hour

// MemberLocation is where a member's device last said it was.
type MemberLocation struct {
	Coords
	// Place is the name of the town it is in, such as "Berlin"; empty when it couldn't be found.
	Place     string
	UpdatedAt time.Time
}

// Fresh reports whether the position is recent enough to forecast from at now.
func (l MemberLocation) Fresh(now time.Time) bool {
	return now.Sub(l.UpdatedAt) <= LocationFreshFor
}
