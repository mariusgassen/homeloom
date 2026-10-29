package domain

import (
	"strings"
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

// Aisle is where in the shop an item is found. The list shows them in the order of Aisles.
type Aisle string

const (
	AisleProduce   Aisle = "produce"
	AisleBakery    Aisle = "bakery"
	AisleMeat      Aisle = "meat"
	AisleDairy     Aisle = "dairy"
	AisleFrozen    Aisle = "frozen"
	AislePantry    Aisle = "pantry"
	AisleDrinks    Aisle = "drinks"
	AisleHousehold Aisle = "household"
	AisleOther     Aisle = "other"
)

// Aisles in the order of a walk through the shop.
var Aisles = []Aisle{AisleProduce, AisleBakery, AisleMeat, AisleDairy, AisleFrozen, AislePantry, AisleDrinks, AisleHousehold, AisleOther}

func (a Aisle) Valid() bool {
	for _, x := range Aisles {
		if a == x {
			return true
		}
	}
	return false
}

// AisleKey is how a household's aisle memory knows a thing again: its text, trimmed and lower-cased.
func AisleKey(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// ShoppingItem is one line on the household's shared shopping list.
type ShoppingItem struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	Text        string
	// Recipe is the name of the recipe the item came from; "" when added by hand.
	Recipe  string
	Aisle   Aisle
	AddedBy *uuid.UUID
	DoneAt  *time.Time
	Created time.Time
}
