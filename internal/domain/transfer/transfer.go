package transfer

import (
	"fmt"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
)

// TransferID is a typed identifier for a transfer aggregate.
type TransferID struct{ id string }

func NewTransferID() TransferID {
	return TransferID{id: newUUID()}
}

func ParseTransferID(s string) (TransferID, error) {
	if err := validateUUID(s); err != nil {
		return TransferID{}, fmt.Errorf("invalid transfer ID %q: %w", s, err)
	}
	return TransferID{id: s}, nil
}

func (t TransferID) String() string { return t.id }

// State represents the lifecycle state of a Transfer.
type State string

const (
	StatePending      State = "PENDING"
	StateDebited      State = "DEBITED"
	StateBankCalled   State = "BANK_CALLED"
	StateCredited     State = "CREDITED"
	StateCompleted    State = "COMPLETED"
	StateFailed       State = "FAILED"
	StateCompensating State = "COMPENSATING"
)

// Transfer is the aggregate root for a money transfer between two accounts.
type Transfer struct {
	ID              TransferID
	IdempotencyKey  string
	SourceAccountID ledger.AccountID
	DestAccountID   ledger.AccountID
	Amount          ledger.Money
	State           State
	ExternalRef     string // reference returned by the bank gateway
	FailureReason   string
	DebitTxID       ledger.TransactionID // set after ReserveDebit
	CreditTxID      ledger.TransactionID // set after PostCredit
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int // optimistic concurrency counter
}

// Transition advances the state machine, returning ErrInvalidStateTransition if illegal.
func (t *Transfer) Transition(next State) error {
	allowed, ok := validTransitions[t.State]
	if !ok {
		return fmt.Errorf("%w: no transitions defined from %s", ErrInvalidStateTransition, t.State)
	}
	for _, v := range allowed {
		if v == next {
			t.State = next
			t.UpdatedAt = time.Now().UTC()
			t.Version++
			return nil
		}
	}
	return fmt.Errorf("%w: %s → %s is not allowed", ErrInvalidStateTransition, t.State, next)
}

// validTransitions encodes the complete state machine.
// Any state not listed as a key is terminal.
var validTransitions = map[State][]State{
	StatePending:      {StateDebited, StateFailed},
	StateDebited:      {StateBankCalled, StateCompensating},
	StateBankCalled:   {StateCredited, StateCompensating},
	StateCredited:     {StateCompleted, StateCompensating},
	StateCompensating: {StateFailed},
}

// New creates a new Transfer in PENDING state.
func New(
	id TransferID,
	idempotencyKey string,
	src, dst ledger.AccountID,
	amount ledger.Money,
) (*Transfer, error) {
	if err := amount.Validate(); err != nil {
		return nil, fmt.Errorf("invalid transfer amount: %w", err)
	}
	if amount.IsZero() {
		return nil, fmt.Errorf("transfer amount must be non-zero")
	}
	if src == dst {
		return nil, fmt.Errorf("source and destination accounts must differ")
	}
	if idempotencyKey == "" {
		return nil, ErrIdempotencyKeyRequired
	}

	now := time.Now().UTC()
	return &Transfer{
		ID:              id,
		IdempotencyKey:  idempotencyKey,
		SourceAccountID: src,
		DestAccountID:   dst,
		Amount:          amount,
		State:           StatePending,
		CreatedAt:       now,
		UpdatedAt:       now,
		Version:         1,
	}, nil
}
