package money

import "errors"

var (
	ErrNegativeAmount    = errors.New("amount must be non-negative")
	ErrCurrencyMismatch  = errors.New("currency mismatch")
	ErrInsufficientFunds = errors.New("insufficient funds")
)
