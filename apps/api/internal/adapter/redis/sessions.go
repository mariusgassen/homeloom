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
	// loginTTL is how long a user has to finish signing in at authentik, Home Assistant or Plex.
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

func (s *Sessions) SaveHomeConnection(ctx context.Context, state string, p domain.PendingHomeConnection) error {
	return s.set(ctx, hashedKey("home-connect", state), p, loginTTL)
}

func (s *Sessions) TakeHomeConnection(ctx context.Context, state string) (domain.PendingHomeConnection, error) {
	var p domain.PendingHomeConnection
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	return p, decode(s.client.GetDel(ctx, hashedKey("home-connect", state)), &p)
}

func (s *Sessions) SaveWatchConnection(ctx context.Context, state string, p domain.PendingWatchConnection) error {
	return s.set(ctx, hashedKey("watch-connect", state), p, loginTTL)
}

func (s *Sessions) TakeWatchConnection(ctx context.Context, state string) (domain.PendingWatchConnection, error) {
	var p domain.PendingWatchConnection
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	return p, decode(s.client.GetDel(ctx, hashedKey("watch-connect", state)), &p)
}

func (s *Sessions) SaveGuestPass(ctx context.Context, token string, p domain.GuestPass) error {
	return s.set(ctx, hashedKey("guest-pass", token), p, domain.GuestPassTTL)
}

func (s *Sessions) GuestPass(ctx context.Context, token string) (domain.GuestPass, error) {
	var p domain.GuestPass
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	return p, decode(s.client.Get(ctx, hashedKey("guest-pass", token)), &p)
}

func (s *Sessions) ClaimGuestPass(ctx context.Context, token, device string) error {
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	sum := sha256.Sum256([]byte(device))
	claim := hex.EncodeToString(sum[:])
	key := hashedKey("guest-claim", token)
	ok, err := s.client.SetNX(ctx, key, claim, domain.GuestPassTTL).Result()
	if err != nil {
		return fmt.Errorf("redis: claim guest pass: %w", err)
	}
	if ok {
		return nil
	}
	owner, err := s.client.Get(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("redis: claim guest pass: %w", err)
	}
	if owner != claim {
		return domain.ErrConflict
	}
	return nil
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

func (s *Sessions) SaveAgentGrant(ctx context.Context, token string, g domain.AgentGrant) error {
	return s.set(ctx, hashedKey("agent", token), g, time.Until(g.ExpiresAt))
}

func (s *Sessions) AgentGrant(ctx context.Context, token string) (domain.AgentGrant, error) {
	var g domain.AgentGrant
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	return g, decode(s.client.Get(ctx, hashedKey("agent", token)), &g)
}

func (s *Sessions) DeleteAgentGrant(ctx context.Context, token string) error {
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	if err := s.client.Del(ctx, hashedKey("agent", token)).Err(); err != nil {
		return fmt.Errorf("redis: delete agent grant: %w", err)
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
