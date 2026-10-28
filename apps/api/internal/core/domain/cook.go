package domain

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

type ShoppingListAdded struct {
	Added int
	List  string
}
