package transfer_test

import (
	"testing"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

func validTransfer(t *testing.T) *transfer.Transfer {
	t.Helper()
	src := ledger.NewAccountID()
	dst := ledger.NewAccountID()
	amount := ledger.MustMoney(10_000, ledger.CurrencyIDR)
	tx, err := transfer.New(transfer.NewTransferID(), "key-"+newUUID(), src, dst, amount)
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}
	return tx
}

func TestTransfer_StateMachine_HappyPath(t *testing.T) {
	tx := validTransfer(t)

	steps := []transfer.State{
		transfer.StateDebited,
		transfer.StateBankCalled,
		transfer.StateCredited,
		transfer.StateCompleted,
	}

	for _, next := range steps {
		if err := tx.Transition(next); err != nil {
			t.Fatalf("transition to %s failed: %v", next, err)
		}
		if tx.State != next {
			t.Errorf("want %s, got %s", next, tx.State)
		}
	}
}

func TestTransfer_StateMachine_InvalidTransition(t *testing.T) {
	tx := validTransfer(t)
	// PENDING → COMPLETED is not a valid single-hop transition.
	if err := tx.Transition(transfer.StateCompleted); err == nil {
		t.Error("expected error for invalid transition PENDING → COMPLETED")
	}
}

func TestTransfer_StateMachine_TerminalStateBlocked(t *testing.T) {
	tx := validTransfer(t)
	tx.Transition(transfer.StateFailed) //nolint:errcheck
	if err := tx.Transition(transfer.StatePending); err == nil {
		t.Error("expected error: terminal state FAILED must not be exited")
	}
}

func TestTransfer_ZeroAmount_Rejected(t *testing.T) {
	src := ledger.NewAccountID()
	dst := ledger.NewAccountID()
	zero := ledger.Money{Amount: 0, Currency: ledger.CurrencyIDR}
	_, err := transfer.New(transfer.NewTransferID(), "key", src, dst, zero)
	if err == nil {
		t.Error("zero amount should be rejected")
	}
}

func TestTransfer_NoIdempotencyKey_Rejected(t *testing.T) {
	src := ledger.NewAccountID()
	dst := ledger.NewAccountID()
	amount := ledger.MustMoney(1000, ledger.CurrencyIDR)
	_, err := transfer.New(transfer.NewTransferID(), "", src, dst, amount)
	if err == nil {
		t.Error("empty idempotency key should be rejected")
	}
}

func TestTransfer_VersionIncrementsOnTransition(t *testing.T) {
	tx := validTransfer(t)
	initialVersion := tx.Version
	tx.Transition(transfer.StateDebited) //nolint:errcheck
	if tx.Version != initialVersion+1 {
		t.Errorf("version should increment on state change: want %d, got %d", initialVersion+1, tx.Version)
	}
}

func newUUID() string {
	return transfer.NewTransferID().String()
}
