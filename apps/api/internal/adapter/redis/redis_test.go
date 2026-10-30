package redis_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/redis"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

type countingRepo struct {
	*memory.Store
	calls int
}

func (c *countingRepo) UpsertByIdentity(ctx context.Context, id domain.Identity) (domain.User, error) {
	c.calls++
	return c.Store.UpsertByIdentity(ctx, id)
}

// Runs against a real Redis when TEST_REDIS_URL is set (CI provides one).
func TestUserCache(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	client, err := redis.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	repo := &countingRepo{Store: memory.NewStore()}
	cache := redis.NewUserCache(repo, client, time.Minute)
	ctx := context.Background()
	id := domain.Identity{Subject: uuid.NewString(), Name: "Alice"}

	first, _ := cache.UpsertByIdentity(ctx, id)
	second, _ := cache.UpsertByIdentity(ctx, id)
	if repo.calls != 1 || first != second {
		t.Fatalf("second call should hit the cache: calls=%d", repo.calls)
	}

	id.Name = "Alice B"
	renamed, _ := cache.UpsertByIdentity(ctx, id)
	if repo.calls != 2 || renamed.Name != "Alice B" || renamed.ID != first.ID {
		t.Fatalf("changed identity should write through: calls=%d %+v", repo.calls, renamed)
	}

	if _, err := cache.SetAvatar(ctx, first.ID, domain.Photo{ContentType: "image/jpeg", Data: []byte{0xFF, 0xD8, 0xFF}}); err != nil {
		t.Fatal(err)
	}
	withAvatar, _ := cache.UpsertByIdentity(ctx, id)
	if repo.calls != 3 || withAvatar.AvatarAt == nil {
		t.Fatalf("a new picture should drop the cached user: calls=%d %+v", repo.calls, withAvatar)
	}
}

func TestUserCacheFallsBackWhenRedisIsDown(t *testing.T) {
	client, _ := redis.Connect("redis://127.0.0.1:1")
	defer client.Close()
	repo := &countingRepo{Store: memory.NewStore()}
	u, err := redis.NewUserCache(repo, client, time.Minute).UpsertByIdentity(context.Background(), domain.Identity{Subject: "s", Name: "A"})
	if err != nil || u.Name != "A" {
		t.Fatalf("want fallback to repository, got %+v %v", u, err)
	}
}
