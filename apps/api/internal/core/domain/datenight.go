package domain

import "time"

const (
	// MaxDatePlace and MaxDateMood bound the free text of a date-night request.
	MaxDatePlace = 100
	MaxDateMood  = 200
	// DateIdeaCount is how many ideas one request offers.
	DateIdeaCount = 3
	// MaxDateAheadDays is how far ahead a date can be planned; forecasts reach about that far.
	MaxDateAheadDays = 14
)

// DateBudget is how much the evening may cost.
type DateBudget string

const (
	DateFree  DateBudget = "free"
	DateEasy  DateBudget = "easy"
	DateTreat DateBudget = "treat"
)

func (b DateBudget) Valid() bool { return b == DateFree || b == DateEasy || b == DateTreat }

// Sky is the weather in a word.
type Sky string

const (
	SkyClear  Sky = "clear"
	SkyCloudy Sky = "cloudy"
	SkyFog    Sky = "fog"
	SkyRain   Sky = "rain"
	SkySnow   Sky = "snow"
	SkyStorm  Sky = "storm"
)

// Forecast is the weather in a place on a day.
type Forecast struct {
	Sky Sky
	// HighC and LowC are degrees Celsius.
	HighC, LowC float64
	// RainPercent is the chance of rain or snow, 0-100.
	RainPercent int
}

// DateQuery is what a couple asks for.
type DateQuery struct {
	// Place is a town or city, as typed.
	Place string
	// Day is the evening, as a calendar date at midnight UTC; the zero value means today.
	Day  time.Time
	Mood string
	// Constraints are limits every idea must respect, such as "in a hotel, no kitchen".
	Constraints string
	Budget      DateBudget
}

// DateBrief is what the LLM is told.
type DateBrief struct {
	DateQuery
	// Forecast is nil when the weather couldn't be fetched.
	Forecast *Forecast
}

// DateIdea is one thing to do together.
type DateIdea struct {
	Title  string
	Reason string
	// Where is a venue or area; empty when it can be anywhere.
	Where string
	// Cost is a short phrase such as "about 30 €"; empty for free.
	Cost string
	// Indoor is whether it stays dry.
	Indoor bool
	// Time is the start as "HH:MM" local time.
	Time string
	// StartsAt is Time on the day asked for, in the household's time zone.
	StartsAt time.Time
}

// DateNight is what a request returns.
type DateNight struct {
	Ideas []DateIdea
	// Forecast is nil when the weather couldn't be fetched.
	Forecast *Forecast
}
