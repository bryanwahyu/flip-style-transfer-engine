package ledger

import (
	"fmt"
	"time"
)

// Posting is a validated double-entry pair: one debit and one credit that sum to zero.
// Callers must use NewPosting — never construct directly.
type Posting struct {
	TransactionID TransactionID
	Debit         LedgerEntry
	Credit        LedgerEntry
}

// NewPosting constructs a balanced double-entry posting.
// src account is debited (money leaves), dst account is credited (money arrives).
func NewPosting(txID TransactionID, src, dst AccountID, amount Money, description string) (Posting, error) {
	if err := amount.Validate(); err != nil {
		return Posting{}, fmt.Errorf("invalid posting amount: %w", err)
	}
	if amount.IsZero() {
		return Posting{}, fmt.Errorf("posting amount must be non-zero")
	}
	if src == dst {
		return Posting{}, fmt.Errorf("source and destination accounts must differ")
	}

	now := time.Now().UTC()
	debit := LedgerEntry{
		ID:            NewLedgerEntryID(),
		TransactionID: txID,
		AccountID:     src,
		Type:          EntryTypeDebit,
		Amount:        amount,
		Description:   description,
		CreatedAt:     now,
	}
	credit := LedgerEntry{
		ID:            NewLedgerEntryID(),
		TransactionID: txID,
		AccountID:     dst,
		Type:          EntryTypeCredit,
		Amount:        amount,
		Description:   description,
		CreatedAt:     now,
	}

	// Invariant: debit + credit must sum to zero in signed representation.
	if debit.SignedAmount()+credit.SignedAmount() != 0 {
		return Posting{}, ErrDoubleEntryViolation
	}

	return Posting{
		TransactionID: txID,
		Debit:         debit,
		Credit:        credit,
	}, nil
}

// Entries returns both ledger entries for persistence.
func (p Posting) Entries() []LedgerEntry {
	return []LedgerEntry{p.Debit, p.Credit}
}

// ReversalPosting creates a reversing (compensating) posting for the original.
// The original debit account becomes the credit, and vice versa.
func (p Posting) ReversalPosting(newTxID TransactionID) (Posting, error) {
	return NewPosting(
		newTxID,
		p.Credit.AccountID, // was credited → now debited to return the money
		p.Debit.AccountID,  // was debited → now credited to restore the balance
		p.Debit.Amount,
		fmt.Sprintf("REVERSAL of %s", p.TransactionID.String()),
	)
}
