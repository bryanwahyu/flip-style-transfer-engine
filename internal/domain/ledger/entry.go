package ledger

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
)

// EntryType distinguishes which side of the ledger an entry sits on.
type EntryType string

const (
	EntryTypeDebit  EntryType = "DEBIT"
	EntryTypeCredit EntryType = "CREDIT"
)

// LedgerEntryID is the typed identity for a single ledger line.
type LedgerEntryID struct{ uuid.UUID }

func NewLedgerEntryID() LedgerEntryID { return LedgerEntryID{uuid.New()} }
func ParseLedgerEntryID(s string) (LedgerEntryID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return LedgerEntryID{}, fmt.Errorf("invalid ledger entry ID %q: %w", s, err)
	}
	return LedgerEntryID{id}, nil
}

// TransactionID groups the two entries that form a balanced double-entry posting.
type TransactionID struct{ uuid.UUID }

func NewTransactionID() TransactionID { return TransactionID{uuid.New()} }
func ParseTransactionID(s string) (TransactionID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return TransactionID{}, fmt.Errorf("invalid transaction ID %q: %w", s, err)
	}
	return TransactionID{id}, nil
}

// LedgerEntry is an immutable record of a financial movement.
// Entries are NEVER updated or deleted. Corrections use reversing entries.
type LedgerEntry struct {
	ID            LedgerEntryID
	TransactionID TransactionID
	AccountID     account.AccountID // semantic owner: account, not ledger
	Type          EntryType
	Amount        money.Money // always positive; sign conveyed by Type
	Description   string
	CreatedAt     time.Time
}

// SignedAmount returns the amount with sign implied by entry type:
// DEBIT is negative (money leaves account), CREDIT is positive.
func (e LedgerEntry) SignedAmount() int64 {
	if e.Type == EntryTypeDebit {
		return -e.Amount.Amount
	}
	return e.Amount.Amount
}
