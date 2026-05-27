-- +migrate Up
-- Transactional outbox: events are written atomically with business data,
-- then relayed to NATS by the outbox-relay process.
CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL   PRIMARY KEY,
    subject      TEXT        NOT NULL,
    payload      BYTEA       NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'PENDING'
                             CHECK (status IN ('PENDING','PUBLISHED')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE INDEX idx_outbox_status     ON outbox (status) WHERE status = 'PENDING';
CREATE INDEX idx_outbox_created_at ON outbox (created_at);

-- +migrate Down
DROP TABLE IF EXISTS outbox;
