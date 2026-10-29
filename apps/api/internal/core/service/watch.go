package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
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
}

// Caps on what one suggestion shows the critic, so a large library still fits a small prompt.
const (
	maxWatchCandidates = 150
	maxWatchSeen       = 40
)

func NewWatch(households port.HouseholdRepository, connections port.WatchConnectionRepository, sessions port.SessionStore, media port.MediaLibrary, critic port.Critic) *Watch {
	return &Watch{households: households, connections: connections, sessions: sessions, media: media, critic: critic}
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
	return domain.Shelf{Connected: true, Libraries: libraries}, nil
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

// everything returns every title in the actor's libraries, or none when Plex isn't set up,
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
