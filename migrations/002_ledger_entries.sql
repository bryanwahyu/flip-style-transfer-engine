-- +migrate Up
CREATE TABLE IF NOT EXISTS ledger_entries (
    id             UUID        PRIMARY KEY,
    transaction_id UUID        NOT NULL,
    account_id     UUID        NOT NULL REFERENCES accounts(id),
    entry_type     TEXT        NOT NULL CHECK (entry_type IN ('DEBIT','CREDIT')),
    amount         BIGINT      NOT NULL CHECK (amount >= 0),
    currency       CHAR(3)     NOT NULL,
    description    TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
    -- No UPDATE or DELETE: corrections happen via reversing entries only.
);

CREATE INDEX idx_ledger_entries_account_id    ON ledger_entries (account_id);
CREATE INDEX idx_ledger_entries_transaction_id ON ledger_entries (transaction_id);
CREATE INDEX idx_ledger_entries_created_at    ON ledger_entries (created_at);

-- +migrate Down
DROP TABLE IF EXISTS ledger_entries;
