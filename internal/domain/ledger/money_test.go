package ledger_test

import (
	"testing"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
)

func TestMoney_Add_SameCurrency(t *testing.T) {
	a := money.Must(100, money.CurrencyIDR)
	b := money.Must(200, money.CurrencyIDR)
	got, err := a.Add(b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Amount != 300 {
		t.Errorf("want 300, got %d", got.Amount)
	}
}

func TestMoney_Add_CurrencyMismatch(t *testing.T) {
	a := money.Must(100, money.CurrencyIDR)
	b := money.Must(100, money.CurrencyUSD)
	_, err := a.Add(b)
	if err == nil {
		t.Error("expected error for currency mismatch")
	}
}

func TestMoney_Sub_InsufficientFunds(t *testing.T) {
	a := money.Must(100, money.CurrencyIDR)
	b := money.Must(200, money.CurrencyIDR)
	_, err := a.Sub(b)
	if err == nil {
		t.Error("expected ErrInsufficientFunds")
	}
}

func TestMoney_NegativeAmount_Rejected(t *testing.T) {
	_, err := money.New(-1, money.CurrencyIDR)
	if err == nil {
		t.Error("negative amount should be rejected")
	}
}
