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

	"github.com/google/uuid"
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
// avatar in authentik misses the cache and is written through immediately. A picture set in
// Homeloom drops the user's cached entries, so it shows on the next request.
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
		// Each user's keys are remembered too, so forget can find them.
		_, err := c.client.TxPipelined(setCtx, func(p goredis.Pipeliner) error {
			p.Set(setCtx, key, raw, c.ttl)
			p.SAdd(setCtx, userKeys(u.ID), key)
			p.Expire(setCtx, userKeys(u.ID), c.ttl)
			return nil
		})
		if err != nil {
			slog.WarnContext(ctx, "user cache write failed", "err", err)
		}
	}
	return u, nil
}

func (c *UserCache) SetAvatar(ctx context.Context, userID uuid.UUID, p domain.Photo) (domain.User, error) {
	u, err := c.next.SetAvatar(ctx, userID, p)
	if err == nil {
		c.forget(ctx, userID)
	}
	return u, err
}

func (c *UserCache) DeleteAvatar(ctx context.Context, userID uuid.UUID) (domain.User, error) {
	u, err := c.next.DeleteAvatar(ctx, userID)
	if err == nil {
		c.forget(ctx, userID)
	}
	return u, err
}

func (c *UserCache) Avatar(ctx context.Context, userID uuid.UUID) (domain.Photo, error) {
	return c.next.Avatar(ctx, userID)
}

func (c *UserCache) SharesHousehold(ctx context.Context, a, b uuid.UUID) (bool, error) {
	return c.next.SharesHousehold(ctx, a, b)
}

// forget drops the user's cached entries. If Redis is down they expire with the TTL instead.
func (c *UserCache) forget(ctx context.Context, userID uuid.UUID) {
	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	keys, err := c.client.SMembers(ctx, userKeys(userID)).Result()
	if err == nil {
		err = c.client.Del(ctx, append(keys, userKeys(userID))...).Err()
	}
	if err != nil {
		slog.WarnContext(ctx, "user cache forget failed", "err", err)
	}
}

func userKeys(id uuid.UUID) string {
	return "homeloom:user-keys:" + id.String()
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
