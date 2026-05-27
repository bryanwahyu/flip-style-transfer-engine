package account

import (
	"fmt"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
)

// Status represents whether an account can participate in transfers.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusFrozen   Status = "FROZEN"
	StatusClosed   Status = "CLOSED"
)

// Account is the customer-facing aggregate. Balance is never stored directly —
// it is always computed from ledger entries to prevent drift.
type Account struct {
	ID        ledger.AccountID
	OwnerName string
	Currency  ledger.Currency
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

func New(id ledger.AccountID, ownerName string, currency ledger.Currency) (*Account, error) {
	if ownerName == "" {
		return nil, fmt.Errorf("owner name is required")
	}
	now := time.Now().UTC()
	return &Account{
		ID:        id,
		OwnerName: ownerName,
		Currency:  currency,
		Status:    StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (a *Account) CanTransact() error {
	switch a.Status {
	case StatusActive:
		return nil
	case StatusFrozen:
		return fmt.Errorf("%w: account %s is frozen", ErrAccountNotTransactable, a.ID)
	case StatusClosed:
		return fmt.Errorf("%w: account %s is closed", ErrAccountNotTransactable, a.ID)
	default:
		return fmt.Errorf("%w: unknown status %s", ErrAccountNotTransactable, a.Status)
	}
}

// ComputeBalance calculates the account balance from a slice of ledger entries.
// This is the ONLY correct way to get an account balance — never cache it as a column.
func ComputeBalance(accountID ledger.AccountID, currency ledger.Currency, entries []ledger.LedgerEntry) ledger.Money {
	var total int64
	for _, e := range entries {
		if e.AccountID == accountID {
			total += e.SignedAmount()
		}
	}
	return ledger.Money{Amount: total, Currency: currency}
}
