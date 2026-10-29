package s3_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/s3"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// Runs against a real store when TEST_S3_ENDPOINT is set, e.g. the homeloom-garage image with
// TEST_S3_ACCESS_KEY_ID and TEST_S3_SECRET_ACCESS_KEY imported and TEST_S3_BUCKET (homeloom).
func TestStore(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT not set")
	}
	bucket := os.Getenv("TEST_S3_BUCKET")
	if bucket == "" {
		bucket = "homeloom"
	}
	s, err := s3.New(endpoint, "garage", bucket, os.Getenv("TEST_S3_ACCESS_KEY_ID"), os.Getenv("TEST_S3_SECRET_ACCESS_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "test/" + uuid.NewString() + ".jpg"
	if _, err := s.GetPhoto(ctx, key); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing photo: want ErrNotFound, got %v", err)
	}
	if err := s.PutPhoto(ctx, key, domain.Photo{ContentType: "image/jpeg", Data: []byte{0xFF, 0xD8, 0xFF, 1}}); err != nil {
		t.Fatal(err)
	}
	if p, err := s.GetPhoto(ctx, key); err != nil || p.ContentType != "image/jpeg" || len(p.Data) != 4 {
		t.Fatalf("photo: %+v, %v", p, err)
	}
	if err := s.DeletePhoto(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPhoto(ctx, key); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted photo: want ErrNotFound, got %v", err)
	}
	if err := s.DeletePhoto(ctx, key); err != nil {
		t.Fatalf("deleting a missing photo: %v", err)
	}
}

func TestNewWantsAURL(t *testing.T) {
	if _, err := s3.New("homeloom-garage:3900", "garage", "homeloom", "id", "secret"); err == nil {
		t.Fatal("want an error for an endpoint without a scheme")
	}
}
