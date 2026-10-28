package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

const (
	// loginTTL is how long a user has to finish signing in at authentik.
	loginTTL = 10 * time.Minute
	// sessionTimeout bounds each call. Sessions have no fallback, so it is more generous than the cache's.
	sessionTimeout = time.Second
)

// Sessions stores pending logins and sessions. Keys hold a hash of the id, not the id itself,
// so a Redis dump does not contain usable cookies.
type Sessions struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewSessions(client *goredis.Client, ttl time.Duration) *Sessions {
	return &Sessions{client: client, ttl: ttl}
}

func (s *Sessions) SaveLogin(ctx context.Context, state string, l domain.PendingLogin) error {
	return s.set(ctx, hashedKey("login", state), l, loginTTL)
}

func (s *Sessions) TakeLogin(ctx context.Context, state string) (domain.PendingLogin, error) {
	var l domain.PendingLogin
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	return l, decode(s.client.GetDel(ctx, hashedKey("login", state)), &l)
}

func (s *Sessions) Save(ctx context.Context, id string, sess domain.Session) error {
	return s.set(ctx, hashedKey("session", id), sess, s.ttl)
}

func (s *Sessions) Get(ctx context.Context, id string) (domain.Session, error) {
	var sess domain.Session
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	return sess, decode(s.client.Get(ctx, hashedKey("session", id)), &sess)
}

func (s *Sessions) Delete(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	if err := s.client.Del(ctx, hashedKey("session", id)).Err(); err != nil {
		return fmt.Errorf("redis: delete session: %w", err)
	}
	return nil
}

func (s *Sessions) set(ctx context.Context, key string, v any, ttl time.Duration) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	if err := s.client.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("redis: save: %w", err)
	}
	return nil
}

func decode(cmd *goredis.StringCmd, v any) error {
	raw, err := cmd.Bytes()
	if errors.Is(err, goredis.Nil) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	return json.Unmarshal(raw, v)
}

func hashedKey(kind, id string) string {
	sum := sha256.Sum256([]byte(id))
	return "homeloom:" + kind + ":" + hex.EncodeToString(sum[:])
}
