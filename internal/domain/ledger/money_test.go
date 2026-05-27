package ledger_test

import (
	"testing"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
)

func TestMoney_Add(t *testing.T) {
	t.Run("same currency", func(t *testing.T) {
		a := ledger.MustMoney(100, ledger.CurrencyIDR)
		b := ledger.MustMoney(200, ledger.CurrencyIDR)
		got, err := a.Add(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Amount != 300 {
			t.Errorf("want 300, got %d", got.Amount)
		}
	})

	t.Run("currency mismatch returns error", func(t *testing.T) {
		a := ledger.MustMoney(100, ledger.CurrencyIDR)
		b := ledger.MustMoney(100, ledger.CurrencyUSD)
		_, err := a.Add(b)
		if err == nil {
			t.Error("expected error for currency mismatch")
		}
	})
}

func TestMoney_Sub(t *testing.T) {
	t.Run("sufficient balance", func(t *testing.T) {
		a := ledger.MustMoney(500, ledger.CurrencyIDR)
		b := ledger.MustMoney(200, ledger.CurrencyIDR)
		got, err := a.Sub(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Amount != 300 {
			t.Errorf("want 300, got %d", got.Amount)
		}
	})

	t.Run("insufficient funds returns error", func(t *testing.T) {
		a := ledger.MustMoney(100, ledger.CurrencyIDR)
		b := ledger.MustMoney(200, ledger.CurrencyIDR)
		_, err := a.Sub(b)
		if err == nil {
			t.Error("expected ErrInsufficientFunds")
		}
	})
}

func TestMoney_NegativeAmount_Rejected(t *testing.T) {
	_, err := ledger.NewMoney(-1, ledger.CurrencyIDR)
	if err == nil {
		t.Error("negative amount should be rejected")
	}
}
