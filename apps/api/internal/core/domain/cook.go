package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	MaxMood = 200
	// SuggestionCount is how many recipes one suggestion offers.
	SuggestionCount = 3
)

type RecipeSummary struct {
	Slug        string
	Name        string
	Description string
	TotalTime   string
	Tags        []string
	// Image is the version of the recipe's picture; "" when it has none.
	Image string
}

// RecipeImage is a recipe's picture, a few hundred pixels wide.
type RecipeImage struct {
	ContentType string
	Body        []byte
}

type Recipe struct {
	RecipeSummary
	Servings    string
	Ingredients []Ingredient
	Steps       []string
	URL         string
}

type Ingredient struct {
	ID   string // Mealie's reference id
	Text string
}

// Pick is a recipe the chef chose, with why it fits; Reason is empty for a random pick.
type Pick struct {
	Slug   string
	Reason string
}

type Suggestions struct {
	Recipes []Suggestion
	// Chef is false when the LLM couldn't be asked and the picks are random.
	Chef bool
}

type Suggestion struct {
	Recipe RecipeSummary
	Reason string
}

const (
	// MaxPlanDays is how many days one read of the meal plan, or one round of ideas, covers.
	MaxPlanDays = 14
	// RecentMealDays is how far back the chef looks, so the plan doesn't repeat last week.
	RecentMealDays = 21
)

// PlannedMeal is a household's dinner on a day. The recipe's summary is kept as it was when it was
// planned, so the plan stays readable while Mealie is down.
type PlannedMeal struct {
	HouseholdID uuid.UUID
	// Day is the calendar date, at midnight UTC.
	Day       time.Time
	Recipe    RecipeSummary
	PlannedBy uuid.UUID
}

// MealIdea is a recipe the chef (or chance) suggests for a day without a dinner.
type MealIdea struct {
	Day    time.Time
	Recipe RecipeSummary
	Reason string
}

type MealIdeas struct {
	Meals []MealIdea
	// Chef is false when the LLM couldn't be asked and the ideas are random.
	Chef bool
}

// RecipeIngredients names some of a recipe's ingredients by their ids.
type RecipeIngredients struct {
	Slug          string
	IngredientIDs []string
}

// Rating is how a member liked a dish.
type Rating string

const (
	RatingLove Rating = "love"
	RatingFine Rating = "fine"
	// RatingNope means not again: the dish is left out of ideas.
	RatingNope Rating = "nope"
)

func (r Rating) Valid() bool { return r == RatingLove || r == RatingFine || r == RatingNope }

// MealRating is one member's rating of a dish. The name is kept as it was when rated, so ratings
// stay readable while Mealie is down.
type MealRating struct {
	HouseholdID uuid.UUID
	UserID      uuid.UUID
	Slug        string
	Name        string
	Rating      Rating
	RatedAt     time.Time
}

const (
	// MaxRecapDishes caps the most-cooked dishes in a recap; MaxRecapPhotos its photos.
	MaxRecapDishes = 5
	MaxRecapPhotos = 12
	// MaxRecapMoments caps how many moments of a year the recap reads.
	MaxRecapMoments = 2000
)

// CookedDish is a dish a household kept memories of.
type CookedDish struct {
	Slug  string
	Name  string
	Times int
	// Last is the day it was cooked last.
	Last time.Time
}

// MealRecap is a year of meal memories.
type MealRecap struct {
	Year int
	// Memories is how many meal memories the viewer can read from the year.
	Memories int
	// Dishes are the most cooked, most first.
	Dishes []CookedDish
	// Favourites are the year's dishes that every member who rated them loved, at least two members.
	Favourites []CookedDish
	// Photos are the newest meal memories with a photo.
	Photos []Moment
}
