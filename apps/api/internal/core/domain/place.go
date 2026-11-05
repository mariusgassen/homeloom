package domain

// MinPlaceQuery is the shortest text a place search starts from.
const MinPlaceQuery = 2

// PlaceMatch is a town or city a search found. Label is how to write it ("Berlin, Germany"): the
// weather lookup finds the same place again from it.
type PlaceMatch struct {
	Label string
}
