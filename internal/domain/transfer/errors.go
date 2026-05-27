package transfer

import "errors"

var (
	ErrInsufficientFunds          = errors.New("insufficient funds")
	ErrTransferAlreadyCompleted   = errors.New("transfer already completed")
	ErrTransferAlreadyFailed      = errors.New("transfer already failed")
	ErrInvalidStateTransition     = errors.New("invalid state transition")
	ErrTransferNotFound           = errors.New("transfer not found")
	ErrIdempotencyKeyRequired     = errors.New("idempotency key is required")
	ErrIdempotencyKeyReused       = errors.New("idempotency_key_reused_with_different_payload")
	ErrAccountNotFound            = errors.New("account not found")
	ErrSameSourceDestination      = errors.New("source and destination accounts must differ")
)
