package domain

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

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
	// Included is false for a library the person left out: it isn't shown or used for ideas.
	Included bool
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
	// URL opens the title in Plex.
	URL string
}

// TonightPick is what a household chose to watch tonight. It keeps the title's name and link, so
// members whose own Plex can't reach the title still see what was picked. It lasts until the
// morning after it was picked.
type TonightPick struct {
	HouseholdID uuid.UUID
	TitleID     string
	Kind        MediaKind
	Name        string
	Year        int
	URL         string
	PickedBy    uuid.UUID
	PickedAt    time.Time
}

const (
	MaxWatchMood = 200
	// WatchIdeaCount is how many ideas one suggestion offers.
	WatchIdeaCount = 4
)

// WatchPick is a title the LLM chose: one of the candidates by TitleID, or one from outside the
// libraries by name, year and kind.
type WatchPick struct {
	TitleID string
	Name    string
	Year    int
	Kind    MediaKind
	Reason  string
}

// WatchIdea is something to watch and why it fits. Title is set when it is in the person's
// libraries.
type WatchIdea struct {
	Name   string
	Year   int
	Kind   MediaKind
	Reason string
	Title  *Title
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
	// Hidden lists the ids of the libraries the person left out.
	Hidden []string
}

// PendingWatchConnection is a Plex sign-in that was started and waits for the person to come back.
// Pin is Plex's id for the sign-in, which turns into a token once they have approved it.
type PendingWatchConnection struct {
	UserID uuid.UUID
	Pin    string
}

// Swipe is one member's answer on movie night: they would watch the film, or not. Key names the
// film across people's Plex accounts; TitleID is the swiper's own id for it.
type Swipe struct {
	HouseholdID uuid.UUID
	UserID      uuid.UUID
	Key         string
	TitleID     string
	Name        string
	Year        int
	Liked       bool
	SwipedAt    time.Time
}

// FilmKey names a film regardless of whose Plex it comes from: case, accents and punctuation
// don't matter.
func FilmKey(name string, year int) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(name) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String() + "." + strconv.Itoa(year)
}

// NightMatch is a film both partners would watch. Title is the asking member's own copy, so they
// can pick it for tonight.
type NightMatch struct {
	Title Title
}

// MovieNight is a member's view of the swiping: films still to swipe on, and the matches so far.
type MovieNight struct {
	Deck    []Title
	Matches []NightMatch
	// Partners is how many other members are swiping; matches need at least one.
	Partners int
}

// MaxNightDeck is how many films one view offers.
const MaxNightDeck = 20
