package domain

import "github.com/google/uuid"

// Scene and Light are the only Home Assistant entities Homeloom can switch. ID is the entity id,
// e.g. scene.dinner or light.kitchen. Room is the id of the room it is in, or empty.
type Scene struct {
	ID   string
	Name string
	Room string
}

type Light struct {
	ID   string
	Name string
	Room string
	On   bool
}

// ReadingKind is what a Reading measures or detects.
type ReadingKind string

const (
	Temperature ReadingKind = "temperature"
	Humidity    ReadingKind = "humidity"
	Power       ReadingKind = "power"
	CO2         ReadingKind = "co2"
	Window      ReadingKind = "window"
	Door        ReadingKind = "door"
	GarageDoor  ReadingKind = "garage_door"
	Motion      ReadingKind = "motion"
	Moisture    ReadingKind = "moisture"
	Smoke       ReadingKind = "smoke"
)

// Measured reports whether the kind has a Value and Unit; the others only have On.
func (k ReadingKind) Measured() bool {
	switch k {
	case Temperature, Humidity, Power, CO2:
		return true
	}
	return false
}

// Reading is a live state that is shown but can never be changed, such as a temperature or an
// open window. On means open, detected or wet.
type Reading struct {
	ID    string
	Name  string
	Room  string
	Kind  ReadingKind
	Value float64
	Unit  string
	On    bool
}

// Devices is what Home Assistant shows Homeloom. Members of a group are left out, so the group
// stands for them.
type Devices struct {
	Scenes   []Scene
	Lights   []Light
	Readings []Reading
}

// Room is a Home Assistant area.
type Room struct {
	ID   string
	Name string
}

// Layout is how the home is arranged in Home Assistant: its rooms, which room each entity is in
// (by entity id), and the entities hidden there or marked as config or diagnostics.
type Layout struct {
	Rooms  []Room
	Areas  map[string]string
	Hidden map[string]bool
}

type Home struct {
	// Connected is false until the person has signed in to Home Assistant; the lists are then empty.
	Connected bool
	// Rooms are the rooms that hold something, by name.
	Rooms []Room
	Devices
}

// HomeConnection is a person's link to their Home Assistant account: the refresh token from
// their sign-in there. It is a secret and is stored encrypted.
type HomeConnection struct {
	UserID       uuid.UUID
	RefreshToken string
}

// PendingHomeConnection is a Home Assistant sign-in that was started and waits for the redirect back.
type PendingHomeConnection struct {
	UserID uuid.UUID
}
