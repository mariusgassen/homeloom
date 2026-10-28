package domain

import "github.com/google/uuid"

// Scene and Light are the only Home Assistant entities Homeloom exposes. ID is the entity id,
// e.g. scene.dinner or light.kitchen.
type Scene struct {
	ID   string
	Name string
}

type Light struct {
	ID   string
	Name string
	On   bool
}

type Home struct {
	// Connected is false until the person has signed in to Home Assistant; the lists are then empty.
	Connected bool
	Scenes    []Scene
	Lights    []Light
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
