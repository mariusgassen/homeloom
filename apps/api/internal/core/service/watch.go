package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Watch reads the movies and series each member can watch in Plex: libraries on their own
// servers and on servers friends share with them. Each member connects their own Plex account, so
// everyone sees what they can reach and their own watched state. Nothing is played or changed.
// Without Plex (media is nil) the feature is unavailable.
type Watch struct {
	households  port.HouseholdRepository
	connections port.WatchConnectionRepository
	sessions    port.SessionStore
	media       port.MediaLibrary
	// critic is nil without an LLM; ideas are then unavailable. They don't need Plex.
	critic port.Critic
	// tonight is nil until WithTonight; picking for tonight is then unavailable.
	tonight port.TonightRepository
	// swipes is nil until WithNight; movie night is then unavailable.
	swipes port.SwipeRepository
	notify notifier
	loc    *time.Location
	now    func() time.Time
}

// tonightEnds is the hour of the morning after a pick when it stops being tonight's.
const tonightEnds = 5

// Caps on what one suggestion shows the critic, so a large library still fits a small prompt.
const (
	maxWatchCandidates = 150
	maxWatchSeen       = 40
)

func NewWatch(households port.HouseholdRepository, connections port.WatchConnectionRepository, sessions port.SessionStore, media port.MediaLibrary, critic port.Critic) *Watch {
	return &Watch{households: households, connections: connections, sessions: sessions, media: media, critic: critic, now: time.Now}
}

// WithTonight lets a household pick what to watch tonight; the others are told through notify,
// and loc says when the night is over.
func (s *Watch) WithTonight(tonight port.TonightRepository, notify notifier, loc *time.Location) *Watch {
	s.tonight, s.notify, s.loc = tonight, notify, loc
	return s
}

// WithNight lets the household's members swipe through films together.
func (s *Watch) WithNight(swipes port.SwipeRepository) *Watch {
	s.swipes = swipes
	return s
}

func (s *Watch) Connect(ctx context.Context, actor domain.User, householdID uuid.UUID) (string, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return "", err
	}
	state := randomID()
	pin, url, err := s.media.StartSignIn(ctx, state)
	if err != nil {
		return "", err
	}
	if err := s.sessions.SaveWatchConnection(ctx, state, domain.PendingWatchConnection{UserID: actor.ID, Pin: pin}); err != nil {
		return "", err
	}
	return url, nil
}

func (s *Watch) CompleteConnect(ctx context.Context, actor domain.User, state string) error {
	if s.media == nil {
		return fmt.Errorf("%w: Plex is not configured", domain.ErrUnavailable)
	}
	if state == "" {
		return fmt.Errorf("%w: missing state", domain.ErrUnauthenticated)
	}
	pending, err := s.sessions.TakeWatchConnection(ctx, state)
	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("%w: unknown or expired state", domain.ErrUnauthenticated)
	}
	if err != nil {
		return err
	}
	// The sign-in must come back to the person who started it.
	if pending.UserID != actor.ID {
		return fmt.Errorf("%w: started by someone else", domain.ErrUnauthenticated)
	}
	token, err := s.media.FinishSignIn(ctx, pending.Pin)
	if err != nil {
		return err
	}
	return s.connections.SaveWatchConnection(ctx, domain.WatchConnection{UserID: actor.ID, Token: token})
}

func (s *Watch) Disconnect(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return err
	}
	return s.connections.DeleteWatchConnection(ctx, actor.ID)
}

func (s *Watch) Shelf(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Shelf, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.Shelf{}, err
	}
	notConnected := domain.Shelf{Libraries: []domain.Library{}}
	c, err := s.connections.WatchConnection(ctx, actor.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return notConnected, nil
	}
	if err != nil {
		return domain.Shelf{}, err
	}
	libraries, err := s.media.Libraries(ctx, c.Token)
	if errors.Is(err, domain.ErrUnauthenticated) {
		// Signed out in Plex: forget it, so the app offers to connect again.
		slog.WarnContext(ctx, "plex rejected the connection", "err", err)
		if err := s.connections.DeleteWatchConnection(ctx, actor.ID); err != nil {
			return domain.Shelf{}, err
		}
		return notConnected, nil
	}
	if err != nil {
		return domain.Shelf{}, err
	}
	for i := range libraries {
		libraries[i].Included = !slices.Contains(c.Hidden, libraries[i].ID)
	}
	return domain.Shelf{Connected: true, Libraries: libraries}, nil
}

// Include puts one of the actor's libraries on their shelf, or leaves it out. It is the actor's
// own choice and changes nothing in Plex; a library they can't reach is domain.ErrNotFound.
func (s *Watch) Include(ctx context.Context, actor domain.User, householdID uuid.UUID, libraryID string, included bool) error {
	token, err := s.token(ctx, actor, householdID)
	if err != nil {
		return err
	}
	libraries, err := s.media.Libraries(ctx, token)
	if err != nil {
		return unavailable(err)
	}
	if !slices.ContainsFunc(libraries, func(l domain.Library) bool { return l.ID == libraryID }) {
		return fmt.Errorf("%w: no such library", domain.ErrNotFound)
	}
	return s.connections.SetWatchLibraryHidden(ctx, actor.ID, libraryID, !included)
}

func (s *Watch) Titles(ctx context.Context, actor domain.User, householdID uuid.UUID, libraryID string) ([]domain.Title, error) {
	token, err := s.token(ctx, actor, householdID)
	if err != nil {
		return nil, err
	}
	titles, err := s.media.Titles(ctx, token, libraryID)
	return titles, unavailable(err)
}

func (s *Watch) Poster(ctx context.Context, actor domain.User, householdID uuid.UUID, titleID string) (domain.Poster, error) {
	token, err := s.token(ctx, actor, householdID)
	if err != nil {
		return domain.Poster{}, err
	}
	p, err := s.media.Poster(ctx, token, titleID)
	return p, unavailable(err)
}

func (s *Watch) Pick(ctx context.Context, actor domain.User, householdID uuid.UUID, titleID string) (domain.TonightPick, error) {
	token, err := s.token(ctx, actor, householdID)
	if err != nil {
		return domain.TonightPick{}, err
	}
	if s.tonight == nil {
		return domain.TonightPick{}, fmt.Errorf("%w: tonight's pick is not set up", domain.ErrUnavailable)
	}
	t, err := s.media.Title(ctx, token, titleID)
	if err != nil {
		return domain.TonightPick{}, unavailable(err)
	}
	p := domain.TonightPick{
		HouseholdID: householdID, TitleID: t.ID, Kind: t.Kind, Name: t.Name, Year: t.Year, URL: t.URL,
		PickedBy: actor.ID, PickedAt: s.now(),
	}
	if err := s.tonight.SaveTonight(ctx, p); err != nil {
		return domain.TonightPick{}, err
	}
	s.notifyPicked(ctx, actor, p)
	return p, nil
}

func (s *Watch) Tonight(ctx context.Context, actor domain.User, householdID uuid.UUID) (*domain.TonightPick, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	if s.tonight == nil {
		return nil, nil
	}
	p, err := s.tonight.Tonight(ctx, householdID)
	if errors.Is(err, domain.ErrNotFound) || err == nil && !s.now().Before(s.nightEnds(p.PickedAt)) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Watch) ClearTonight(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	if s.tonight == nil {
		return nil
	}
	return s.tonight.DeleteTonight(ctx, householdID)
}

// nightEnds is the next tonightEnds o'clock after a pick: a pick at 1 a.m. is still that night's.
func (s *Watch) nightEnds(pickedAt time.Time) time.Time {
	t := pickedAt.In(s.loc)
	y, m, d := t.Date()
	end := time.Date(y, m, d, tonightEnds, 0, 0, 0, s.loc)
	if !t.Before(end) {
		end = end.AddDate(0, 0, 1)
	}
	return end
}

// notifyPicked tells the other members what is on tonight.
func (s *Watch) notifyPicked(ctx context.Context, actor domain.User, p domain.TonightPick) {
	members, err := s.households.Members(ctx, p.HouseholdID)
	if err != nil {
		slog.WarnContext(ctx, "tonight's pick: list members", "err", err)
		return
	}
	var others []uuid.UUID
	for _, m := range members {
		if m.User.ID != actor.ID {
			others = append(others, m.User.ID)
		}
	}
	s.notify.Notify(ctx, others, domain.Notification{
		Key:  "push.watch_tonight",
		Args: []any{"name", actor.Name, "title", p.Name},
		URL:  "/watch",
		Tag:  "watch-tonight-" + p.HouseholdID.String(),
	})
}

func (s *Watch) Suggest(ctx context.Context, actor domain.User, householdID uuid.UUID, mood, lang string) ([]domain.WatchIdea, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	if s.critic == nil {
		return nil, fmt.Errorf("%w: no LLM is configured", domain.ErrUnavailable)
	}
	mood = strings.TrimSpace(mood)
	if utf8.RuneCountInString(mood) > domain.MaxWatchMood {
		return nil, fmt.Errorf("%w: mood must be at most %d characters", domain.ErrInvalid, domain.MaxWatchMood)
	}
	var unseen, seen []domain.Title
	for _, t := range s.everything(ctx, actor) {
		if t.Watched {
			seen = append(seen, t)
		} else {
			unseen = append(unseen, t)
		}
	}
	unseen, seen = sample(unseen, maxWatchCandidates), sample(seen, maxWatchSeen)
	picks, err := s.critic.Pick(ctx, mood, lang, unseen, seen, domain.WatchIdeaCount)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.Title, len(unseen))
	for _, t := range unseen {
		byID[t.ID] = t
	}
	ideas := make([]domain.WatchIdea, 0, len(picks))
	for _, p := range picks {
		if len(ideas) == domain.WatchIdeaCount {
			break
		}
		if t, ok := byID[p.TitleID]; ok {
			delete(byID, p.TitleID)
			ideas = append(ideas, domain.WatchIdea{Name: t.Name, Year: t.Year, Kind: t.Kind, Reason: p.Reason, Title: &t})
			continue
		}
		name := strings.TrimSpace(p.Name)
		if p.TitleID != "" || name == "" {
			// An id that wasn't offered is made up; drop it.
			continue
		}
		kind := p.Kind
		if kind != domain.Shows {
			kind = domain.Movies
		}
		ideas = append(ideas, domain.WatchIdea{Name: name, Year: p.Year, Kind: kind, Reason: p.Reason})
	}
	if len(ideas) == 0 {
		return nil, fmt.Errorf("%w: the LLM suggested nothing usable", domain.ErrUnavailable)
	}
	return ideas, nil
}

// everything returns every title in the libraries the actor includes, or none when Plex isn't set up,
// connected or reachable: ideas then come from outside the libraries only.
func (s *Watch) everything(ctx context.Context, actor domain.User) []domain.Title {
	if s.media == nil {
		return nil
	}
	c, err := s.connections.WatchConnection(ctx, actor.ID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			slog.WarnContext(ctx, "watch ideas: read connection", "err", err)
		}
		return nil
	}
	libraries, err := s.media.Libraries(ctx, c.Token)
	if err != nil {
		slog.WarnContext(ctx, "watch ideas: plex libraries", "err", err)
		return nil
	}
	var all []domain.Title
	for _, l := range libraries {
		if slices.Contains(c.Hidden, l.ID) {
			continue
		}
		titles, err := s.media.Titles(ctx, c.Token, l.ID)
		if err != nil {
			slog.WarnContext(ctx, "watch ideas: plex titles", "library", l.ID, "err", err)
			continue
		}
		all = append(all, titles...)
	}
	return all
}

// sample returns up to n of titles, chosen at random.
func sample(titles []domain.Title, n int) []domain.Title {
	rand.Shuffle(len(titles), func(i, j int) { titles[i], titles[j] = titles[j], titles[i] }) //nolint:gosec // picking a film, not a secret
	return titles[:min(n, len(titles))]
}

// token returns the actor's Plex token; without a connection Plex is unavailable.
func (s *Watch) token(ctx context.Context, actor domain.User, householdID uuid.UUID) (string, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return "", err
	}
	c, err := s.connections.WatchConnection(ctx, actor.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", fmt.Errorf("%w: Plex is not connected", domain.ErrUnavailable)
	}
	return c.Token, err
}

// ready checks membership first, so non-members learn nothing about whether Plex is set up.
func (s *Watch) ready(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	if s.media == nil {
		return fmt.Errorf("%w: Plex is not configured", domain.ErrUnavailable)
	}
	return nil
}

// Movie night: each member swipes through the unseen films in their own libraries, and a film
// both like is a match. Films are compared by name and year, so it works across Plex accounts and
// servers. Swipes last until the morning, like tonight's pick.

// nightStart is the last tonightEnds o'clock: swipes before it belong to an earlier night.
func (s *Watch) nightStart() time.Time {
	now := s.now().In(s.loc)
	y, m, d := now.Date()
	start := time.Date(y, m, d, tonightEnds, 0, 0, 0, s.loc)
	if now.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start
}

func (s *Watch) Night(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.MovieNight, error) {
	if err := s.nightReady(ctx, actor, householdID); err != nil {
		return domain.MovieNight{}, err
	}
	swipes, err := s.swipes.Swipes(ctx, householdID, s.nightStart())
	if err != nil {
		return domain.MovieNight{}, err
	}
	mine := map[string]domain.Swipe{}
	theirs := map[string]bool{}
	partners := map[uuid.UUID]bool{}
	for _, w := range swipes {
		if w.UserID == actor.ID {
			mine[w.Key] = w
		} else if w.Liked {
			theirs[w.Key] = true
		}
	}
	members, err := s.households.Members(ctx, householdID)
	if err != nil {
		return domain.MovieNight{}, err
	}
	for _, m := range members {
		if m.User.ID != actor.ID {
			partners[m.User.ID] = true
		}
	}
	night := domain.MovieNight{Deck: []domain.Title{}, Matches: []domain.NightMatch{}, Partners: len(partners)}
	var liked, rest []domain.Title
	for _, t := range s.everything(ctx, actor) {
		if t.Kind != domain.Movies || t.Watched {
			continue
		}
		key := domain.FilmKey(t.Name, t.Year)
		switch w, swiped := mine[key]; {
		case swiped && w.Liked && theirs[key]:
			night.Matches = append(night.Matches, domain.NightMatch{Title: t})
		case swiped:
		case theirs[key]:
			liked = append(liked, t)
		default:
			rest = append(rest, t)
		}
	}
	// What a partner liked comes first, so matches turn up quickly.
	rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] }) //nolint:gosec // picking a film, not a secret
	night.Deck = append(liked, rest...)
	night.Deck = night.Deck[:min(domain.MaxNightDeck, len(night.Deck))]
	return night, nil
}

func (s *Watch) Swipe(ctx context.Context, actor domain.User, householdID uuid.UUID, titleID string, liked bool) (*domain.NightMatch, error) {
	token, err := s.token(ctx, actor, householdID)
	if err != nil {
		return nil, err
	}
	if s.swipes == nil {
		return nil, fmt.Errorf("%w: movie night is not set up", domain.ErrUnavailable)
	}
	t, err := s.media.Title(ctx, token, titleID)
	if err != nil {
		return nil, unavailable(err)
	}
	if t.Kind != domain.Movies {
		return nil, fmt.Errorf("%w: not a movie", domain.ErrNotFound)
	}
	key := domain.FilmKey(t.Name, t.Year)
	err = s.swipes.SaveSwipe(ctx, domain.Swipe{
		HouseholdID: householdID, UserID: actor.ID, Key: key, TitleID: t.ID, Name: t.Name, Year: t.Year, Liked: liked, SwipedAt: s.now(),
	})
	if err != nil || !liked {
		return nil, err
	}
	swipes, err := s.swipes.Swipes(ctx, householdID, s.nightStart())
	if err != nil {
		return nil, err
	}
	for _, w := range swipes {
		if w.UserID != actor.ID && w.Key == key && w.Liked {
			s.notifyMatch(ctx, actor, householdID, t)
			return &domain.NightMatch{Title: t}, nil
		}
	}
	return nil, nil
}

func (s *Watch) ResetNight(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if err := s.nightReady(ctx, actor, householdID); err != nil {
		return err
	}
	return s.swipes.DeleteSwipes(ctx, householdID)
}

// nightReady checks membership first, then that movie night is set up.
func (s *Watch) nightReady(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	if s.swipes == nil || s.media == nil {
		return fmt.Errorf("%w: movie night is not set up", domain.ErrUnavailable)
	}
	return nil
}

// notifyMatch tells the partner who swiped earlier that they agree on a film.
func (s *Watch) notifyMatch(ctx context.Context, actor domain.User, householdID uuid.UUID, t domain.Title) {
	members, err := s.households.Members(ctx, householdID)
	if err != nil {
		slog.WarnContext(ctx, "movie night: list members", "err", err)
		return
	}
	var others []uuid.UUID
	for _, m := range members {
		if m.User.ID != actor.ID {
			others = append(others, m.User.ID)
		}
	}
	s.notify.Notify(ctx, others, domain.Notification{
		Key: "push.watch_match", Args: []any{"title", t.Name}, URL: "/watch/night", Tag: "watch-match-" + householdID.String(),
	})
}
