package ledger_test

import (
	"testing"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
)

func TestNewPosting_DoubleEntry(t *testing.T) {
	src := account.NewAccountID()
	dst := account.NewAccountID()
	amount := money.Must(100_000, money.CurrencyIDR)
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
	id := account.NewAccountID()
	amount := money.Must(100, money.CurrencyIDR)
	_, err := ledger.NewPosting(ledger.NewTransactionID(), id, id, amount, "self-transfer")
	if err == nil {
		t.Error("expected error for same source and destination")
	}
}

func TestNewPosting_ZeroAmountReturnsError(t *testing.T) {
	src, dst := account.NewAccountID(), account.NewAccountID()
	zero := money.Money{Amount: 0, Currency: money.CurrencyIDR}
	_, err := ledger.NewPosting(ledger.NewTransactionID(), src, dst, zero, "zero")
	if err == nil {
		t.Error("expected error for zero amount")
	}
}

func TestPosting_Reversal_SwapsAccounts(t *testing.T) {
	src, dst := account.NewAccountID(), account.NewAccountID()
	amount := money.Must(50_000, money.CurrencyIDR)
	txID := ledger.NewTransactionID()

	original, err := ledger.NewPosting(txID, src, dst, amount, "original")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reversal, err := original.Reversal(ledger.NewTransactionID())
	if err != nil {
		t.Fatalf("reversal error: %v", err)
	}

	if reversal.Debit.AccountID != dst {
		t.Errorf("reversal debit should be original credit account %s, got %s", dst, reversal.Debit.AccountID)
	}
	if reversal.Credit.AccountID != src {
		t.Errorf("reversal credit should be original debit account %s, got %s", src, reversal.Credit.AccountID)
	}
}
