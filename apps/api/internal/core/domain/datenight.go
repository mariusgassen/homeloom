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

// Condition is the weather described more closely than Sky, from the forecast's weather code.
type Condition string

const (
	CondClear        Condition = "clear"
	CondMostlyClear  Condition = "mostlyClear"
	CondPartlyCloudy Condition = "partlyCloudy"
	CondOvercast     Condition = "overcast"
	CondFog          Condition = "fog"
	CondDrizzle      Condition = "drizzle"
	CondRain         Condition = "rain"
	CondFreezingRain Condition = "freezingRain"
	CondShowers      Condition = "showers"
	CondSnow         Condition = "snow"
	CondSnowShowers  Condition = "snowShowers"
	CondThunderstorm Condition = "thunderstorm"
)

// Forecast is the weather in a place on a day.
type Forecast struct {
	// Place is the name of the town the forecast is for, such as "Berlin".
	Place     string
	Sky       Sky
	Condition Condition
	// HighC and LowC are degrees Celsius.
	HighC, LowC float64
	// RainPercent is the chance of rain or snow, 0-100.
	RainPercent int
}

// DateQuery is what a couple asks for.
type DateQuery struct {
	// Place is a town or city, as typed.
	Place string
	// Location is where the device is. It wins over Place for the forecast, and the town it is in
	// stands in for an empty Place.
	Location *Coords
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
