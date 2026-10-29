package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
}

func NewWatch(households port.HouseholdRepository, connections port.WatchConnectionRepository, sessions port.SessionStore, media port.MediaLibrary) *Watch {
	return &Watch{households: households, connections: connections, sessions: sessions, media: media}
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
