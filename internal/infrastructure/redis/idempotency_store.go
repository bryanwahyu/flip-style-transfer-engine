package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// IdempotencyStore implements port.IdempotencyStore using Redis.
// Keys are namespaced with "idempotency:" to avoid collisions.
type IdempotencyStore struct {
	client *goredis.Client
}

func NewIdempotencyStore(client *goredis.Client) *IdempotencyStore {
	return &IdempotencyStore{client: client}
}

func (s *IdempotencyStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := s.client.Get(ctx, s.prefixed(key)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("redis get %s: %w", key, err)
	}
	return val, true, nil
}

func (s *IdempotencyStore) SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	ok, err := s.client.SetNX(ctx, s.prefixed(key), value, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis setnx %s: %w", key, err)
	}
	return ok, nil
}

func (s *IdempotencyStore) prefixed(key string) string {
	return "idempotency:" + key
}
