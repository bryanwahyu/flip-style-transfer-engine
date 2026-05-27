package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxWriter implements port.OutboxWriter.
// It writes events to the outbox table within the caller's database transaction,
// ensuring atomic business write + event publication intent.
type OutboxWriter struct {
	db *pgxpool.Pool
}

func NewOutboxWriter(db *pgxpool.Pool) *OutboxWriter {
	return &OutboxWriter{db: db}
}

func (w *OutboxWriter) Write(ctx context.Context, subject string, payload []byte) error {
	_, err := w.db.Exec(ctx, `
		INSERT INTO outbox (subject, payload, created_at, status)
		VALUES ($1, $2, $3, 'PENDING')`,
		subject, payload, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("write to outbox: %w", err)
	}
	return nil
}
