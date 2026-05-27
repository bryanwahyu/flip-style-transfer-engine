package ledger

import (
	"fmt"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
)

// Posting is a validated double-entry pair: one debit and one credit summing to zero.
// Construct only via NewPosting — never build directly.
type Posting struct {
	TransactionID TransactionID
	Debit         LedgerEntry
	Credit        LedgerEntry
}

// NewPosting creates a balanced posting: src is debited, dst is credited.
func NewPosting(txID TransactionID, src, dst account.AccountID, amount money.Money, description string) (Posting, error) {
	if err := amount.Validate(); err != nil {
		return Posting{}, fmt.Errorf("posting amount invalid: %w", err)
	}
	if amount.IsZero() {
		return Posting{}, fmt.Errorf("posting amount must be non-zero")
	}
	if src == dst {
		return Posting{}, fmt.Errorf("source and destination accounts must differ")
	}

	now := time.Now().UTC()
	debit := LedgerEntry{
		ID: NewLedgerEntryID(), TransactionID: txID, AccountID: src,
		Type: EntryTypeDebit, Amount: amount, Description: description, CreatedAt: now,
	}
	credit := LedgerEntry{
		ID: NewLedgerEntryID(), TransactionID: txID, AccountID: dst,
		Type: EntryTypeCredit, Amount: amount, Description: description, CreatedAt: now,
	}

	// Invariant enforced at construction, not by discipline.
	if debit.SignedAmount()+credit.SignedAmount() != 0 {
		return Posting{}, ErrDoubleEntryViolation
	}

	return Posting{TransactionID: txID, Debit: debit, Credit: credit}, nil
}

// Entries returns both ledger entries for persistence.
func (p Posting) Entries() []LedgerEntry { return []LedgerEntry{p.Debit, p.Credit} }

// Reversal creates a compensating posting that undoes the original.
// The original debit account is credited, and vice versa.
func (p Posting) Reversal(newTxID TransactionID) (Posting, error) {
	return NewPosting(
		newTxID,
		p.Credit.AccountID, // was credited → now debited (returns the money)
		p.Debit.AccountID,  // was debited → now credited (restores the balance)
		p.Debit.Amount,
		fmt.Sprintf("REVERSAL of tx %s", p.TransactionID.String()),
	)
}
