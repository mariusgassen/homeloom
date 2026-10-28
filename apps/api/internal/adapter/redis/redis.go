// Package redis holds Redis-backed adapters: caching in front of slower ports, and health.
package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

func Connect(url string) (*goredis.Client, error) {
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}
	return goredis.NewClient(opts), nil
}

// UserCache sits in front of a UserRepository so that an authenticated request does not
// write to Postgres every time. The key covers every identity field, so a changed name or
// avatar in authentik misses the cache and is written through immediately.
// cacheTimeout bounds each Redis call, so a Redis outage costs a request milliseconds, not seconds.
const cacheTimeout = 250 * time.Millisecond

type UserCache struct {
	next   port.UserRepository
	client *goredis.Client
	ttl    time.Duration
}

func NewUserCache(next port.UserRepository, client *goredis.Client, ttl time.Duration) *UserCache {
	return &UserCache{next: next, client: client, ttl: ttl}
}

func (c *UserCache) UpsertByIdentity(ctx context.Context, id domain.Identity) (domain.User, error) {
	key := identityKey(id)
	getCtx, cancel := context.WithTimeout(ctx, cacheTimeout)
	raw, err := c.client.Get(getCtx, key).Bytes()
	cancel()
	if err == nil {
		var u domain.User
		if json.Unmarshal(raw, &u) == nil {
			return u, nil
		}
	} else if !errors.Is(err, goredis.Nil) {
		// Redis being down must not lock anyone out; fall through to the repository.
		slog.WarnContext(ctx, "user cache read failed", "err", err)
	}

	u, err := c.next.UpsertByIdentity(ctx, id)
	if err != nil {
		return u, err
	}
	if raw, err := json.Marshal(u); err == nil {
		setCtx, cancel := context.WithTimeout(ctx, cacheTimeout)
		defer cancel()
		if err := c.client.Set(setCtx, key, raw, c.ttl).Err(); err != nil {
			slog.WarnContext(ctx, "user cache write failed", "err", err)
		}
	}
	return u, nil
}

func identityKey(id domain.Identity) string {
	h := sha256.New()
	for _, f := range []string{id.Subject, id.Name, id.Email, id.AvatarURL} {
		h.Write([]byte(f))
		h.Write([]byte{0})
	}
	return "homeloom:user:" + hex.EncodeToString(h.Sum(nil))
}

// HealthCheck reports whether Redis answers.
type HealthCheck struct{ Client *goredis.Client }

func (HealthCheck) Name() string                      { return "redis" }
func (h HealthCheck) Check(ctx context.Context) error { return h.Client.Ping(ctx).Err() }
