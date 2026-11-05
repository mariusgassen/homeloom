package service

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Wishes is the household's wishlist. Every member sees and changes all of it, rates each wish
// in hearts, and the wishes wanted most come first.
type Wishes struct {
	households port.HouseholdRepository
	wishes     port.WishRepository
	// photos keeps the pictures; nil means pictures aren't available.
	photos port.PhotoStore
	now    func() time.Time
}

func NewWishes(households port.HouseholdRepository, wishes port.WishRepository) *Wishes {
	return &Wishes{households: households, wishes: wishes, now: time.Now}
}

// WithPhotos keeps pictures of wishes in photos.
func (s *Wishes) WithPhotos(photos port.PhotoStore) *Wishes {
	s.photos = photos
	return s
}

func (s *Wishes) List(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Wish, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	ws, err := s.wishes.Wishes(ctx, householdID, domain.MaxWishes)
	if err != nil {
		return nil, err
	}
	// Stable: wishes with as many hearts stay newest first.
	slices.SortStableFunc(ws, func(a, b domain.Wish) int {
		if (a.GotAt != nil) != (b.GotAt != nil) {
			if a.GotAt != nil {
				return 1
			}
			return -1
		}
		return b.Hearts() - a.Hearts()
	})
	return ws, nil
}

func (s *Wishes) Create(ctx context.Context, actor domain.User, householdID uuid.UUID, f domain.WishFields) (domain.Wish, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Wish{}, err
	}
	f, err := checkWish(f)
	if err != nil {
		return domain.Wish{}, err
	}
	return s.wishes.CreateWish(ctx, domain.Wish{HouseholdID: householdID, WishFields: f, CreatedBy: &actor.ID})
}

func (s *Wishes) Update(ctx context.Context, actor domain.User, householdID, id uuid.UUID, f domain.WishFields) (domain.Wish, error) {
	w, err := s.get(ctx, actor, householdID, id)
	if err != nil {
		return domain.Wish{}, err
	}
	if w.WishFields, err = checkWish(f); err != nil {
		return domain.Wish{}, err
	}
	return s.wishes.UpdateWish(ctx, w)
}

func (s *Wishes) Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error {
	w, err := s.get(ctx, actor, householdID, id)
	if err != nil {
		return err
	}
	// The photo goes first, so a deleted wish never leaves its picture behind.
	if w.PhotoAt != nil {
		if s.photos == nil {
			return storeDown(domain.ErrUnavailable)
		}
		if err := s.photos.DeletePhoto(ctx, wishKey(id)); err != nil {
			return storeDown(err)
		}
	}
	return s.wishes.DeleteWish(ctx, householdID, id)
}

func (s *Wishes) Vote(ctx context.Context, actor domain.User, householdID, id uuid.UUID, hearts int) (domain.Wish, error) {
	if _, err := s.get(ctx, actor, householdID, id); err != nil {
		return domain.Wish{}, err
	}
	if hearts < 0 || hearts > domain.MaxWishHearts {
		return domain.Wish{}, domain.ErrWishHearts
	}
	return s.wishes.VoteWish(ctx, householdID, id, actor.ID, hearts)
}

func (s *Wishes) Got(ctx context.Context, actor domain.User, householdID, id uuid.UUID, got bool) (domain.Wish, error) {
	if _, err := s.get(ctx, actor, householdID, id); err != nil {
		return domain.Wish{}, err
	}
	var at *time.Time
	if got {
		now := s.now()
		at = &now
	}
	return s.wishes.SetWishGot(ctx, householdID, id, at)
}

func (s *Wishes) SetPhoto(ctx context.Context, actor domain.User, householdID, id uuid.UUID, p domain.Photo) (domain.Wish, error) {
	if _, err := s.photoOf(ctx, actor, householdID, id); err != nil {
		return domain.Wish{}, err
	}
	if p.ContentType != "image/jpeg" || len(p.Data) > domain.MaxPhotoBytes || !bytes.HasPrefix(p.Data, []byte{0xFF, 0xD8, 0xFF}) {
		return domain.Wish{}, domain.ErrWishPhoto
	}
	if err := s.photos.PutPhoto(ctx, wishKey(id), p); err != nil {
		return domain.Wish{}, storeDown(err)
	}
	now := s.now()
	return s.wishes.SetWishPhoto(ctx, householdID, id, &now)
}

func (s *Wishes) Photo(ctx context.Context, actor domain.User, householdID, id uuid.UUID) (domain.Photo, error) {
	w, err := s.photoOf(ctx, actor, householdID, id)
	if err != nil {
		return domain.Photo{}, err
	}
	if w.PhotoAt == nil {
		return domain.Photo{}, domain.ErrNotFound
	}
	p, err := s.photos.GetPhoto(ctx, wishKey(id))
	if err != nil {
		return domain.Photo{}, storeDown(err)
	}
	return p, nil
}

func (s *Wishes) DeletePhoto(ctx context.Context, actor domain.User, householdID, id uuid.UUID) (domain.Wish, error) {
	w, err := s.photoOf(ctx, actor, householdID, id)
	if err != nil {
		return domain.Wish{}, err
	}
	if w.PhotoAt == nil {
		return w, nil
	}
	if err := s.photos.DeletePhoto(ctx, wishKey(id)); err != nil {
		return domain.Wish{}, storeDown(err)
	}
	return s.wishes.SetWishPhoto(ctx, householdID, id, nil)
}

// get returns the wish if the actor is a member of its household.
func (s *Wishes) get(ctx context.Context, actor domain.User, householdID, id uuid.UUID) (domain.Wish, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Wish{}, err
	}
	return s.wishes.GetWish(ctx, householdID, id)
}

// photoOf is get, and fails when pictures aren't available.
func (s *Wishes) photoOf(ctx context.Context, actor domain.User, householdID, id uuid.UUID) (domain.Wish, error) {
	w, err := s.get(ctx, actor, householdID, id)
	if err != nil {
		return domain.Wish{}, err
	}
	if s.photos == nil {
		return domain.Wish{}, fmt.Errorf("%w: wish photos are not set up", domain.ErrUnavailable)
	}
	return w, nil
}

// checkWish validates and normalises what was asked for a wish.
func checkWish(f domain.WishFields) (domain.WishFields, error) {
	f.Title, f.Note, f.URL = strings.TrimSpace(f.Title), strings.TrimSpace(f.Note), strings.TrimSpace(f.URL)
	switch {
	case f.Title == "" || utf8.RuneCountInString(f.Title) > domain.MaxWishTitle:
		return f, domain.TextError{Max: domain.MaxWishTitle}
	case utf8.RuneCountInString(f.Note) > domain.MaxWishNote:
		return f, domain.ErrWishNote
	case f.URL != "" && (len(f.URL) > domain.MaxWishURL || !domain.WebLink(f.URL)):
		return f, domain.ErrWishLink
	case f.PriceCents != nil && (*f.PriceCents < 1 || *f.PriceCents > domain.MaxWishPriceCents):
		return f, domain.ErrWishPrice
	}
	return f, nil
}

// wishKey is where a wish's picture is kept in the PhotoStore.
func wishKey(id uuid.UUID) string { return "wishes/" + id.String() + ".jpg" }
