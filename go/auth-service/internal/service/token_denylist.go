package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// TokenDenylist stores revoked token hashes in Redis with TTL.
// If Redis is nil, revocation is a no-op (tokens expire naturally).
type TokenDenylist struct {
	client *redis.Client
	prefix string
}

// NewTokenDenylist creates a denylist. Pass nil to disable revocation.
func NewTokenDenylist(client *redis.Client) *TokenDenylist {
	return &TokenDenylist{client: client, prefix: "auth:denied"}
}

// Revoke adds a token to the denylist with the given TTL.
func (d *TokenDenylist) Revoke(ctx context.Context, token string, ttl time.Duration) error {
	if d.client == nil {
		return nil
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
	return d.client.Set(ctx, d.prefix+":"+hash, "1", ttl).Err()
}

// IsRevoked checks if a token has been revoked.
func (d *TokenDenylist) IsRevoked(ctx context.Context, token string) bool {
	if d.client == nil {
		return false
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
	val, err := d.client.Get(ctx, d.prefix+":"+hash).Result()
	if err != nil {
		return false // Fail open on Redis errors
	}
	return val == "1"
}
