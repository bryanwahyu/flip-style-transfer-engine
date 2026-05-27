package ledger

import (
	"fmt"
)

// Currency is an ISO-4217 currency code.
type Currency string

const (
	CurrencyIDR Currency = "IDR"
	CurrencyUSD Currency = "USD"
)

// Money is an immutable value object representing an amount in a specific currency.
// Amount is stored as the smallest unit (e.g., cents for USD, rupiah for IDR).
type Money struct {
	Amount   int64
	Currency Currency
}

func NewMoney(amount int64, currency Currency) (Money, error) {
	if amount < 0 {
		return Money{}, ErrNegativeAmount
	}
	return Money{Amount: amount, Currency: currency}, nil
}

func MustMoney(amount int64, currency Currency) Money {
	m, err := NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return m
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s and %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}
	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s and %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}
	if m.Amount < other.Amount {
		return Money{}, ErrInsufficientFunds
	}
	return Money{Amount: m.Amount - other.Amount, Currency: m.Currency}, nil
}

func (m Money) Negate() Money {
	return Money{Amount: -m.Amount, Currency: m.Currency}
}

func (m Money) IsZero() bool { return m.Amount == 0 }

func (m Money) Equals(other Money) bool {
	return m.Amount == other.Amount && m.Currency == other.Currency
}

func (m Money) String() string {
	return fmt.Sprintf("%s %d", m.Currency, m.Amount)
}

func (m Money) Validate() error {
	if m.Amount < 0 {
		return ErrNegativeAmount
	}
	if m.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	return nil
}
