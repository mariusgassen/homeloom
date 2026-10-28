package domain

// Call is what a client needs to join a household's call: the LiveKit server it connects to and
// an access token for the room.
type Call struct {
	URL   string
	Token string
	Room  string
}
