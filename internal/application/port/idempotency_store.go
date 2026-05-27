package port

import (
	"context"
	"time"
)

// IdempotencyStore persists request fingerprints to detect and deduplicate retries.
// Backed by Redis with a 24-hour TTL.
type IdempotencyStore interface {
	// Get retrieves a previously cached response. Returns (nil, false, nil) on miss.
	Get(ctx context.Context, key string) ([]byte, bool, error)

	// SetIfAbsent stores value only if key does not exist (first-writer-wins).
	// Returns true if stored (first call), false if key already existed (retry).
	SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
}
