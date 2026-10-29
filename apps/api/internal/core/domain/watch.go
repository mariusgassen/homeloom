package domain

import "github.com/google/uuid"

// MediaKind is what a library holds.
type MediaKind string

const (
	Movies MediaKind = "movie"
	Shows  MediaKind = "show"
)

// Library is a movie or series library on a Plex server the person owns or that is shared with
// them. ID names the server and the library, so it is only meaningful to that person's account.
type Library struct {
	ID     string
	Name   string
	Kind   MediaKind
	Server string
	// Owned is false for a library a friend shares.
	Owned bool
}

// Title is a movie or a series in a library. Minutes is a movie's length, or an episode's for a
// series; Watched means the person has seen the movie, or every episode.
type Title struct {
	ID            string
	Kind          MediaKind
	Name          string
	Year          int
	Summary       string
	Minutes       int
	Genres        []string
	Rating        float64
	ContentRating string
	Watched       bool
	// Episodes is the number of episodes of a series; 0 for a movie.
	Episodes int
}

// Shelf is what the person can watch: every movie and series library they can reach.
type Shelf struct {
	// Connected is false until the person has signed in to Plex; Libraries is then empty.
	Connected bool
	Libraries []Library
}

// Poster is a title's cover image.
type Poster struct {
	ContentType string
	Body        []byte
}

// WatchConnection is a person's link to their Plex account: the token from signing in there. It
// is a secret and is stored encrypted.
type WatchConnection struct {
	UserID uuid.UUID
	Token  string
}

// PendingWatchConnection is a Plex sign-in that was started and waits for the person to come back.
// Pin is Plex's id for the sign-in, which turns into a token once they have approved it.
type PendingWatchConnection struct {
	UserID uuid.UUID
	Pin    string
}
