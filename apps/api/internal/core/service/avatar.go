package service

import (
	"bytes"
	"context"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Avatars keeps the pictures members set for themselves. Each person changes only their own,
// and only the people they share a household with see it.
type Avatars struct {
	users port.UserRepository
}

func NewAvatars(users port.UserRepository) *Avatars {
	return &Avatars{users: users}
}

func (s *Avatars) Set(ctx context.Context, actor domain.User, p domain.Photo) (domain.User, error) {
	// The app sends JPEGs; checking the bytes as well keeps anything else from being served as one.
	if p.ContentType != "image/jpeg" || len(p.Data) > domain.MaxPhotoBytes || !bytes.HasPrefix(p.Data, []byte{0xFF, 0xD8, 0xFF}) {
		return domain.User{}, domain.ErrAvatar
	}
	return s.users.SetAvatar(ctx, actor.ID, p)
}

func (s *Avatars) Delete(ctx context.Context, actor domain.User) (domain.User, error) {
	return s.users.DeleteAvatar(ctx, actor.ID)
}

func (s *Avatars) Get(ctx context.Context, actor domain.User, userID uuid.UUID) (domain.Photo, error) {
	if userID != actor.ID {
		ok, err := s.users.SharesHousehold(ctx, actor.ID, userID)
		if err != nil {
			return domain.Photo{}, err
		}
		if !ok {
			return domain.Photo{}, domain.ErrNotFound
		}
	}
	return s.users.Avatar(ctx, userID)
}
