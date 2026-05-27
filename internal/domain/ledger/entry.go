package ledger

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EntryType distinguishes the side of the ledger an entry sits on.
type EntryType string

const (
	EntryTypeDebit  EntryType = "DEBIT"
	EntryTypeCredit EntryType = "CREDIT"
)

// LedgerEntryID is a typed identifier for a ledger entry.
type LedgerEntryID struct{ uuid.UUID }

func NewLedgerEntryID() LedgerEntryID { return LedgerEntryID{uuid.New()} }
func ParseLedgerEntryID(s string) (LedgerEntryID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return LedgerEntryID{}, fmt.Errorf("invalid ledger entry ID %q: %w", s, err)
	}
	return LedgerEntryID{id}, nil
}

// TransactionID groups the two (or more) entries that form a double-entry posting.
type TransactionID struct{ uuid.UUID }

func NewTransactionID() TransactionID { return TransactionID{uuid.New()} }
func ParseTransactionID(s string) (TransactionID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return TransactionID{}, fmt.Errorf("invalid transaction ID %q: %w", s, err)
	}
	return TransactionID{id}, nil
}

// AccountID is a typed identifier for a ledger account.
// Defined here (not in the account package) so ledger stays self-contained.
type AccountID struct{ uuid.UUID }

func NewAccountID() AccountID { return AccountID{uuid.New()} }
func ParseAccountID(s string) (AccountID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return AccountID{}, fmt.Errorf("invalid account ID %q: %w", s, err)
	}
	return AccountID{id}, nil
}

// LedgerEntry is an immutable record of a financial movement on an account.
// Entries are NEVER updated or deleted — corrections happen via reversing entries.
type LedgerEntry struct {
	ID            LedgerEntryID
	TransactionID TransactionID
	AccountID     AccountID
	Type          EntryType
	Amount        Money   // always positive; sign is conveyed by Type
	Description   string
	CreatedAt     time.Time
}

// SignedAmount returns the amount with the sign implied by entry type:
// debits are negative (money leaves account), credits are positive.
func (e LedgerEntry) SignedAmount() int64 {
	if e.Type == EntryTypeDebit {
		return -e.Amount.Amount
	}
	return e.Amount.Amount
}
