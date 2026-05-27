// Package port defines the interfaces (ports) between the application and infrastructure layers.
// Following ISP, each interface declares only what its consumer actually needs.
package port

import (
	"context"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

// ── Transfer ─────────────────────────────────────────────────────────────────

// TransferReader is used by query handlers and the HTTP layer.
type TransferReader interface {
	FindByID(ctx context.Context, id transfer.TransferID) (*transfer.Transfer, error)
	FindByIdempotencyKey(ctx context.Context, key string) (*transfer.Transfer, error)
}

// TransferWriter is used by command handlers and the saga.
type TransferWriter interface {
	Save(ctx context.Context, t *transfer.Transfer) error
	UpdateState(ctx context.Context, t *transfer.Transfer) error
}

// TransferRepository is the composite for callers that need both sides.
type TransferRepository interface {
	TransferReader
	TransferWriter
}

// ── Account ──────────────────────────────────────────────────────────────────

// AccountReader is used by command handlers that only read account state.
type AccountReader interface {
	FindByID(ctx context.Context, id account.AccountID) (*account.Account, error)
}

// AccountLocker is used by the saga to acquire a row-level lock before debit,
// preventing concurrent double-spends from the same account.
type AccountLocker interface {
	LockForUpdate(ctx context.Context, id account.AccountID) (*account.Account, error)
}

// AccountRepository is the composite used by the admin/seed path.
type AccountRepository interface {
	AccountReader
	AccountLocker
	Save(ctx context.Context, a *account.Account) error
}

// ── Ledger ───────────────────────────────────────────────────────────────────

// EntryWriter is used by the saga to post debit/credit entries.
type EntryWriter interface {
	PostEntries(ctx context.Context, entries []ledger.LedgerEntry) error
}

// BalanceReader is used by the saga (pre-debit check) and the HTTP balance endpoint.
type BalanceReader interface {
	GetBalance(ctx context.Context, accountID account.AccountID, currency money.Currency) (money.Money, error)
}

// LedgerAuditor is used exclusively by the reconciler.
// Intentionally separate from EntryWriter/BalanceReader so the reconciler
// cannot accidentally post entries.
type LedgerAuditor interface {
	GetAllEntries(ctx context.Context) ([]ledger.LedgerEntry, error)
}

// ── Events & Outbox ──────────────────────────────────────────────────────────

// TransferEventStore records every state-change event for audit and replay.
type TransferEventStore interface {
	Append(ctx context.Context, event transfer.TransferEvent) error
	FindByTransferID(ctx context.Context, id transfer.TransferID) ([]transfer.TransferEvent, error)
}

// OutboxWriter atomically enqueues an event alongside the business write,
// within the same database transaction.
type OutboxWriter interface {
	Write(ctx context.Context, subject string, payload []byte) error
}
