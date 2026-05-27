// Package account owns the Account aggregate and its identity type.
// AccountID lives here — not in ledger — because accounts exist independently
// of any ledger mechanics.
package account

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
)

// AccountID is the typed identity of an account.
// It is defined here so that ledger and transfer can reference it without
// creating circular dependencies.
type AccountID struct{ uuid.UUID }

func NewAccountID() AccountID                 { return AccountID{uuid.New()} }
func (a AccountID) String() string            { return a.UUID.String() }
func ParseAccountID(s string) (AccountID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return AccountID{}, fmt.Errorf("invalid account ID %q: %w", s, err)
	}
	return AccountID{id}, nil
}

// FloatAccounts holds the system clearing accounts per currency.
// These are seeded on first run and have well-known, stable IDs.
var FloatAccounts = map[money.Currency]AccountID{
	money.CurrencyIDR: {uuid.MustParse("00000000-0000-0000-0000-000000000001")},
	money.CurrencyUSD: {uuid.MustParse("00000000-0000-0000-0000-000000000002")},
}

// Status represents whether an account may participate in transfers.
type Status string

const (
	StatusActive Status = "ACTIVE"
	StatusFrozen Status = "FROZEN"
	StatusClosed Status = "CLOSED"
)

// Account is the customer-facing aggregate.
// Balance is never stored on this struct — it is always derived from ledger entries.
type Account struct {
	ID        AccountID
	OwnerName string
	Currency  money.Currency
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

func New(id AccountID, ownerName string, currency money.Currency) (*Account, error) {
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
	default:
		return fmt.Errorf("%w: account %s has status %s", ErrAccountNotTransactable, a.ID, a.Status)
	}
}
