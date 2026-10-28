package domain

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
	Scenes []Scene
	Lights []Light
}
