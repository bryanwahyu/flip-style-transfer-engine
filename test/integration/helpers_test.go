package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
)

// testDB connects to the integration test database.
// Set DATABASE_URL env var; defaults to the docker-compose service.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://flip:flip@localhost:5432/flip?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v — is docker compose up?", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// testRedis connects to the integration test Redis.
func testRedis(t *testing.T) *goredis.Client {
	t.Helper()
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	client := goredis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping redis: %v — is docker compose up?", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// seedAccount inserts a test account and returns its ID.
func seedAccount(t *testing.T, db *pgxpool.Pool, ownerName, currency string) string {
	t.Helper()
	id := newUUID()
	_, err := db.Exec(context.Background(), `
		INSERT INTO accounts (id, owner_name, currency, status, created_at, updated_at)
		VALUES ($1, $2, $3, 'ACTIVE', NOW(), NOW())`,
		id, ownerName, currency,
	)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return id
}

// seedBalance credits an account directly via ledger entries (for test setup only).
func seedBalance(t *testing.T, db *pgxpool.Pool, accountID string, amount int64, currency string) {
	t.Helper()
	entryID := newUUID()
	txID := newUUID()
	// Credit the account from the system float account.
	_, err := db.Exec(context.Background(), `
		INSERT INTO ledger_entries (id, transaction_id, account_id, entry_type, amount, currency, description, created_at)
		VALUES ($1, $2, $3, 'CREDIT', $4, $5, 'test seed balance', NOW())`,
		entryID, txID, accountID, amount, currency,
	)
	if err != nil {
		t.Fatalf("seed balance: %v", err)
	}
}

func newUUID() string {
	// Simple UUID v4 without importing uuid package in test helpers.
	// Use the google/uuid package instead:
	return mustUUID()
}

func mustUUID() string {
	f, err := os.Open("/dev/urandom")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	b := make([]byte, 16)
	f.Read(b) //nolint:errcheck
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
