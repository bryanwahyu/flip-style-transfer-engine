-- +migrate Up
-- Append-only event log for every transfer state transition.
-- Enables full audit trail and event sourcing.
CREATE TABLE IF NOT EXISTS transfer_events (
    id          UUID        PRIMARY KEY,
    transfer_id UUID        NOT NULL REFERENCES transfers(id),
    event_type  TEXT        NOT NULL,
    state       TEXT        NOT NULL,
    amount      BIGINT      NOT NULL,
    currency    CHAR(3)     NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata    JSONB       NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_transfer_events_transfer_id  ON transfer_events (transfer_id);
CREATE INDEX idx_transfer_events_occurred_at  ON transfer_events (occurred_at);
CREATE INDEX idx_transfer_events_event_type   ON transfer_events (event_type);

-- +migrate Down
DROP TABLE IF EXISTS transfer_events;
