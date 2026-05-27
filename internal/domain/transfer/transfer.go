// Package transfer owns the Transfer aggregate — the core business concept of
// moving money between two accounts. It deliberately does NOT import the ledger
// package: how funds are tracked in the ledger is an application-layer concern,
// not a transfer domain concern.
package transfer

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
)

// TransferID is the typed identity of a transfer aggregate.
type TransferID struct{ uuid.UUID }

func NewTransferID() TransferID { return TransferID{uuid.New()} }
func (t TransferID) String() string { return t.UUID.String() }
func ParseTransferID(s string) (TransferID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return TransferID{}, fmt.Errorf("invalid transfer ID %q: %w", s, err)
	}
	return TransferID{id}, nil
}

// State is the lifecycle state of a Transfer.
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

// Transfer is the aggregate root. It owns the state machine and all invariants
// for a single money-movement request.
type Transfer struct {
	ID              TransferID
	IdempotencyKey  string
	SourceAccountID account.AccountID
	DestAccountID   account.AccountID
	Amount          money.Money
	State           State
	ExternalRef     string
	FailureReason   string
	// Flags track which ledger legs were posted so compensation knows what to reverse.
	DebitPosted  bool
	CreditPosted bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int
}

// validTransitions encodes the complete state machine.
// Any state absent from this map is terminal.
var validTransitions = map[State][]State{
	StatePending:      {StateDebited, StateFailed},
	StateDebited:      {StateBankCalled, StateCompensating},
	StateBankCalled:   {StateCredited, StateCompensating},
	StateCredited:     {StateCompleted, StateCompensating},
	StateCompensating: {StateFailed},
}

// Transition advances the state machine, returning ErrInvalidStateTransition for illegal moves.
func (t *Transfer) Transition(next State) error {
	for _, allowed := range validTransitions[t.State] {
		if allowed == next {
			t.State = next
			t.UpdatedAt = time.Now().UTC()
			t.Version++
			return nil
		}
	}
	return fmt.Errorf("%w: %s → %s", ErrInvalidStateTransition, t.State, next)
}

// New creates a Transfer in PENDING state.
func New(id TransferID, idempotencyKey string, src, dst account.AccountID, amount money.Money) (*Transfer, error) {
	if idempotencyKey == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	if err := amount.Validate(); err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}
	if amount.IsZero() {
		return nil, fmt.Errorf("transfer amount must be non-zero")
	}
	if src == dst {
		return nil, fmt.Errorf("source and destination accounts must differ")
	}

	now := time.Now().UTC()
	return &Transfer{
		ID: id, IdempotencyKey: idempotencyKey,
		SourceAccountID: src, DestAccountID: dst,
		Amount: amount, State: StatePending,
		CreatedAt: now, UpdatedAt: now, Version: 1,
	}, nil
}
