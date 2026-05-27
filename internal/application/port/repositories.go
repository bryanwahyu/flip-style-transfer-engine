package port

import (
	"context"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

// TransferRepository persists and retrieves Transfer aggregates.
type TransferRepository interface {
	Save(ctx context.Context, t *transfer.Transfer) error
	FindByID(ctx context.Context, id transfer.TransferID) (*transfer.Transfer, error)
	FindByIdempotencyKey(ctx context.Context, key string) (*transfer.Transfer, error)
	UpdateState(ctx context.Context, t *transfer.Transfer) error
}

// AccountRepository reads account records. Balance is always derived from ledger.
type AccountRepository interface {
	FindByID(ctx context.Context, id ledger.AccountID) (*account.Account, error)
	Save(ctx context.Context, a *account.Account) error
	// LockForUpdate acquires a row-level lock within the caller's transaction,
	// preventing concurrent double-spends.
	LockForUpdate(ctx context.Context, id ledger.AccountID) (*account.Account, error)
}

// LedgerRepository posts entries and computes balances from the immutable entry log.
type LedgerRepository interface {
	PostEntries(ctx context.Context, entries []ledger.LedgerEntry) error
	GetBalance(ctx context.Context, accountID ledger.AccountID, currency ledger.Currency) (ledger.Money, error)
	// GetAllEntries is used by the reconciler to detect drift.
	GetAllEntries(ctx context.Context) ([]ledger.LedgerEntry, error)
}

// OutboxWriter atomically enqueues an event alongside the business write,
// within the same database transaction.
type OutboxWriter interface {
	Write(ctx context.Context, subject string, payload []byte) error
}

// TransferEventStore records every state-change event for auditability.
type TransferEventStore interface {
	Append(ctx context.Context, event transfer.TransferEvent) error
	FindByTransferID(ctx context.Context, id transfer.TransferID) ([]transfer.TransferEvent, error)
}
