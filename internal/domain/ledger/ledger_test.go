package ledger_test

import (
	"testing"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
)

func TestNewPosting_DoubleEntry(t *testing.T) {
	src := ledger.NewAccountID()
	dst := ledger.NewAccountID()
	amount := ledger.MustMoney(100_000, ledger.CurrencyIDR)
	txID := ledger.NewTransactionID()

	posting, err := ledger.NewPosting(txID, src, dst, amount, "test posting")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := posting.Entries()
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}

	var sum int64
	for _, e := range entries {
		sum += e.SignedAmount()
	}
	if sum != 0 {
		t.Errorf("double-entry invariant violated: signed sum = %d (must be 0)", sum)
	}
}

func TestNewPosting_SameAccountReturnsError(t *testing.T) {
	id := ledger.NewAccountID()
	amount := ledger.MustMoney(100, ledger.CurrencyIDR)
	_, err := ledger.NewPosting(ledger.NewTransactionID(), id, id, amount, "self-transfer")
	if err == nil {
		t.Error("expected error for same source and destination account")
	}
}

func TestNewPosting_ZeroAmountReturnsError(t *testing.T) {
	src := ledger.NewAccountID()
	dst := ledger.NewAccountID()
	zero := ledger.Money{Amount: 0, Currency: ledger.CurrencyIDR}
	_, err := ledger.NewPosting(ledger.NewTransactionID(), src, dst, zero, "zero amount")
	if err == nil {
		t.Error("expected error for zero amount posting")
	}
}

func TestPosting_Reversal(t *testing.T) {
	src := ledger.NewAccountID()
	dst := ledger.NewAccountID()
	amount := ledger.MustMoney(50_000, ledger.CurrencyIDR)
	txID := ledger.NewTransactionID()

	original, err := ledger.NewPosting(txID, src, dst, amount, "original")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reversal, err := original.ReversalPosting(ledger.NewTransactionID())
	if err != nil {
		t.Fatalf("reversal error: %v", err)
	}

	// In the reversal, debit and credit accounts should be swapped.
	if reversal.Debit.AccountID != dst {
		t.Errorf("reversal debit should be original credit account (%s), got %s",
			dst.String(), reversal.Debit.AccountID.String())
	}
	if reversal.Credit.AccountID != src {
		t.Errorf("reversal credit should be original debit account (%s), got %s",
			src.String(), reversal.Credit.AccountID.String())
	}
}
