package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Bucket is the household's someday list of things to watch, read or visit. Every member sees, adds,
// ticks off and deletes the same bucket.
type Bucket struct {
	households port.HouseholdRepository
	bucket     port.BucketRepository
}

func NewBucket(households port.HouseholdRepository, bucket port.BucketRepository) *Bucket {
	return &Bucket{households: households, bucket: bucket}
}

func (s *Bucket) List(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.BucketKind) ([]domain.BucketItem, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	if kind != "" && !kind.Valid() {
		return nil, domain.ErrBucketKind
	}
	return s.bucket.BucketItems(ctx, householdID, kind, domain.MaxBucketItems)
}

func (s *Bucket) Add(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.BucketKind, text string) (domain.BucketItem, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.BucketItem{}, err
	}
	if !kind.Valid() {
		return domain.BucketItem{}, domain.ErrBucketKind
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > domain.MaxBucketText {
		return domain.BucketItem{}, domain.TextError{Max: domain.MaxBucketText}
	}
	return s.bucket.AddBucketItem(ctx, domain.BucketItem{HouseholdID: householdID, Kind: kind, Text: text, AddedBy: &actor.ID})
}

func (s *Bucket) SetDone(ctx context.Context, actor domain.User, householdID, id uuid.UUID, done bool) (domain.BucketItem, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.BucketItem{}, err
	}
	return s.bucket.SetBucketItemDone(ctx, householdID, id, done)
}

func (s *Bucket) Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	return s.bucket.DeleteBucketItem(ctx, householdID, id)
}
