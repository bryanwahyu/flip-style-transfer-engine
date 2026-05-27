package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	natspkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/nats"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
)

// outbox row represents a pending event in the outbox table.
type outboxRow struct {
	ID      int64
	Subject string
	Payload []byte
}

func main() {
	log := observability.NewLogger()
	if err := run(log); err != nil {
		log.Error("outbox-relay fatal error", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbURL := envOrDefault("DATABASE_URL", "postgres://flip:flip@localhost:5432/flip?sslmode=disable")
	natsURL := envOrDefault("NATS_URL", "nats://localhost:4222")
	pollInterval := 500 * time.Millisecond
	batchSize := 100

	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("connect to nats: %w", err)
	}
	defer nc.Drain() //nolint:errcheck

	publisher, err := natspkg.NewPublisher(nc)
	if err != nil {
		return fmt.Errorf("create publisher: %w", err)
	}

	log.Info("outbox relay started", "poll_interval_ms", pollInterval.Milliseconds())

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := relay(ctx, db, publisher, batchSize, log); err != nil {
				log.Error("outbox relay error", "error", err)
			}
		}
	}
}

// relay polls the outbox table, publishes pending events to NATS, and marks them processed.
// If the relay crashes mid-publish, unacknowledged entries will be re-published on restart
// (at-least-once delivery guarantee).
func relay(ctx context.Context, db *pgxpool.Pool, publisher *natspkg.Publisher, batchSize int, log *slog.Logger) error {
	rows, err := db.Query(ctx, `
		SELECT id, subject, payload
		FROM outbox
		WHERE status = 'PENDING'
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`,
		batchSize,
	)
	if err != nil {
		return fmt.Errorf("query outbox: %w", err)
	}

	var pending []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(&row.ID, &row.Subject, &row.Payload); err != nil {
			rows.Close()
			return fmt.Errorf("scan outbox row: %w", err)
		}
		pending = append(pending, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if len(pending) == 0 {
		return nil
	}

	published := 0
	for _, row := range pending {
		if err := publisher.Publish(ctx, row.Subject, row.Payload); err != nil {
			log.Warn("outbox: publish failed", "id", row.ID, "subject", row.Subject, "error", err)
			continue
		}
		// Mark as published only after successful NATS ack.
		_, err := db.Exec(ctx, `
			UPDATE outbox SET status = 'PUBLISHED', published_at = NOW()
			WHERE id = $1`, row.ID)
		if err != nil {
			log.Warn("outbox: mark published failed", "id", row.ID, "error", err)
			continue
		}
		published++
	}

	if published > 0 {
		log.Info("outbox relay: published events", "count", published)
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
