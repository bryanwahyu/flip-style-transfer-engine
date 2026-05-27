package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
)

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := envOrDefault("DATABASE_URL", "postgres://flip:flip@localhost:5432/flip?sslmode=disable")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect postgres: %v — is docker compose up?", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v — is docker compose up?", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func testRedis(t *testing.T) *goredis.Client {
	t.Helper()
	opts, err := goredis.ParseURL(envOrDefault("REDIS_URL", "redis://localhost:6379"))
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

func seedAccount(t *testing.T, db *pgxpool.Pool, ownerName, currency string) string {
	t.Helper()
	id := newUUID()
	_, err := db.Exec(context.Background(),
		`INSERT INTO accounts (id, owner_name, currency, status, created_at, updated_at)
		 VALUES ($1,$2,$3,'ACTIVE',NOW(),NOW())`,
		id, ownerName, currency,
	)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return id
}

func seedBalance(t *testing.T, db *pgxpool.Pool, accountID string, amount int64, currency string) {
	t.Helper()
	_, err := db.Exec(context.Background(),
		`INSERT INTO ledger_entries (id, transaction_id, account_id, entry_type, amount, currency, description, created_at)
		 VALUES ($1,$2,$3,'CREDIT',$4,$5,'test seed',NOW())`,
		newUUID(), newUUID(), accountID, amount, currency,
	)
	if err != nil {
		t.Fatalf("seed balance: %v", err)
	}
}

func mustAccountID(t *testing.T, s string) account.AccountID {
	t.Helper()
	id, err := account.ParseAccountID(s)
	if err != nil {
		t.Fatalf("parse account id %q: %v", s, err)
	}
	return id
}

func newUUID() string { return uuid.New().String() }

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
