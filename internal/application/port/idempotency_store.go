package port

import (
	"context"
	"errors"
	"time"
)

var ErrIdempotencyKeyConflict = errors.New("idempotency key already used with different payload")

// IdempotencyStore persists request fingerprints to detect duplicates.
// The store is backed by Redis with a 24-hour TTL.
type IdempotencyStore interface {
	// Get retrieves a previously cached response for this key.
	// Returns (nil, false, nil) on cache miss.
	Get(ctx context.Context, key string) ([]byte, bool, error)

	// SetIfAbsent stores the value only if the key does not exist.
	// Returns true if the value was stored (first time), false if it already existed.
	SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
}
