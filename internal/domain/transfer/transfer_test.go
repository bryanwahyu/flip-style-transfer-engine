package transfer_test

import (
	"testing"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

func newValidTransfer(t *testing.T) *transfer.Transfer {
	t.Helper()
	src := account.NewAccountID()
	dst := account.NewAccountID()
	amount := money.Must(10_000, money.CurrencyIDR)
	tx, err := transfer.New(transfer.NewTransferID(), "key-"+transfer.NewTransferID().String(), src, dst, amount)
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}
	return tx
}

func TestTransfer_StateMachine_HappyPath(t *testing.T) {
	tx := newValidTransfer(t)
	for _, next := range []transfer.State{
		transfer.StateDebited, transfer.StateBankCalled,
		transfer.StateCredited, transfer.StateCompleted,
	} {
		if err := tx.Transition(next); err != nil {
			t.Fatalf("transition to %s failed: %v", next, err)
		}
	}
	if tx.State != transfer.StateCompleted {
		t.Errorf("want COMPLETED, got %s", tx.State)
	}
}

func TestTransfer_InvalidTransition_Rejected(t *testing.T) {
	tx := newValidTransfer(t)
	// PENDING → COMPLETED skips steps — must be rejected.
	if err := tx.Transition(transfer.StateCompleted); err == nil {
		t.Error("expected error for illegal transition PENDING → COMPLETED")
	}
}

func TestTransfer_TerminalState_CannotTransition(t *testing.T) {
	tx := newValidTransfer(t)
	tx.Transition(transfer.StateFailed) //nolint:errcheck
	if err := tx.Transition(transfer.StatePending); err == nil {
		t.Error("terminal state FAILED must not allow outgoing transitions")
	}
}

func TestTransfer_ZeroAmount_Rejected(t *testing.T) {
	src, dst := account.NewAccountID(), account.NewAccountID()
	_, err := transfer.New(transfer.NewTransferID(), "key", src, dst, money.Money{Amount: 0, Currency: money.CurrencyIDR})
	if err == nil {
		t.Error("zero amount should be rejected")
	}
}

func TestTransfer_EmptyIdempotencyKey_Rejected(t *testing.T) {
	src, dst := account.NewAccountID(), account.NewAccountID()
	_, err := transfer.New(transfer.NewTransferID(), "", src, dst, money.Must(1000, money.CurrencyIDR))
	if err == nil {
		t.Error("empty idempotency key should be rejected")
	}
}

func TestTransfer_VersionIncrementsOnTransition(t *testing.T) {
	tx := newValidTransfer(t)
	v := tx.Version
	tx.Transition(transfer.StateDebited) //nolint:errcheck
	if tx.Version != v+1 {
		t.Errorf("version should increment: want %d, got %d", v+1, tx.Version)
	}
}
