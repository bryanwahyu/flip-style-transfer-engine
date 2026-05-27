-- +migrate Up
-- Deduplication table for NATS consumer idempotency.
-- Before processing a message, the consumer checks this table.
-- Duplicate events are safely ignored.
CREATE TABLE IF NOT EXISTS processed_events (
    event_id     TEXT        PRIMARY KEY,
    consumer     TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_processed_events_consumer     ON processed_events (consumer);
CREATE INDEX idx_processed_events_processed_at ON processed_events (processed_at);

-- +migrate Down
DROP TABLE IF EXISTS processed_events;
