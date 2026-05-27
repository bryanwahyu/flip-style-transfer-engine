// Package money defines the Money value object used across all domain packages.
// It is the only package allowed to depend on nothing within the domain.
package money

import "fmt"

// Currency is an ISO-4217 currency code.
type Currency string

const (
	CurrencyIDR Currency = "IDR"
	CurrencyUSD Currency = "USD"
)

// Money is an immutable value object. Amount is stored in the smallest unit
// (e.g., rupiah for IDR — no decimal, no float rounding errors).
type Money struct {
	Amount   int64
	Currency Currency
}

func New(amount int64, currency Currency) (Money, error) {
	if amount < 0 {
		return Money{}, ErrNegativeAmount
	}
	return Money{Amount: amount, Currency: currency}, nil
}

func Must(amount int64, currency Currency) Money {
	m, err := New(amount, currency)
	if err != nil {
		panic(err)
	}
	return m
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}
	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}
	if m.Amount < other.Amount {
		return Money{}, ErrInsufficientFunds
	}
	return Money{Amount: m.Amount - other.Amount, Currency: m.Currency}, nil
}

func (m Money) IsZero() bool    { return m.Amount == 0 }
func (m Money) Equals(o Money) bool { return m.Amount == o.Amount && m.Currency == o.Currency }
func (m Money) String() string  { return fmt.Sprintf("%s %d", m.Currency, m.Amount) }

func (m Money) Validate() error {
	if m.Amount < 0 {
		return ErrNegativeAmount
	}
	if m.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	return nil
}
