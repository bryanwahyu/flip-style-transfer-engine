-- +migrate Up
CREATE TABLE IF NOT EXISTS transfers (
    id                UUID        PRIMARY KEY,
    idempotency_key   TEXT        NOT NULL UNIQUE,
    source_account_id UUID        NOT NULL REFERENCES accounts(id),
    dest_account_id   UUID        NOT NULL REFERENCES accounts(id),
    amount            BIGINT      NOT NULL CHECK (amount > 0),
    currency          CHAR(3)     NOT NULL,
    state             TEXT        NOT NULL DEFAULT 'PENDING'
                                  CHECK (state IN (
                                      'PENDING','DEBITED','BANK_CALLED',
                                      'CREDITED','COMPLETED','FAILED','COMPENSATING'
                                  )),
    external_ref      TEXT        NOT NULL DEFAULT '',
    failure_reason    TEXT        NOT NULL DEFAULT '',
    debit_tx_id       UUID,
    credit_tx_id      UUID,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version           INT         NOT NULL DEFAULT 1
);

CREATE INDEX idx_transfers_source_account  ON transfers (source_account_id);
CREATE INDEX idx_transfers_dest_account    ON transfers (dest_account_id);
CREATE INDEX idx_transfers_state           ON transfers (state);
CREATE INDEX idx_transfers_idempotency_key ON transfers (idempotency_key);
CREATE INDEX idx_transfers_created_at      ON transfers (created_at);

-- +migrate Down
DROP TABLE IF EXISTS transfers;
