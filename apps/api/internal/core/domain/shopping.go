package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	MaxShoppingText = 200
	// MaxShoppingItems is how many items the list shows; one request adds at most this many.
	MaxShoppingItems = 500
)

// MaxSuggestions is how many past titles a suggestion list holds.
const MaxSuggestions = 20

// TitleSuggestion is a title used before, to use again, and how often it was used.
type TitleSuggestion struct {
	Text string
	Uses int
}

// ShoppingItem is one line on the household's shared shopping list.
type ShoppingItem struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	Text        string
	// Recipe is the name of the recipe the item came from; "" when added by hand.
	Recipe  string
	AddedBy *uuid.UUID
	DoneAt  *time.Time
	Created time.Time
}
