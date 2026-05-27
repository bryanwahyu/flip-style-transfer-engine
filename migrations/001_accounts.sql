-- +migrate Up
CREATE TABLE IF NOT EXISTS accounts (
    id          UUID        PRIMARY KEY,
    owner_name  TEXT        NOT NULL,
    currency    CHAR(3)     NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'ACTIVE'
                            CHECK (status IN ('ACTIVE','FROZEN','CLOSED')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_accounts_status ON accounts (status);

-- +migrate Down
DROP TABLE IF EXISTS accounts;
