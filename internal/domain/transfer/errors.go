package transfer

import "errors"

var (
	ErrTransferNotFound           = errors.New("transfer not found")
	ErrInvalidStateTransition     = errors.New("invalid state transition")
	ErrIdempotencyKeyRequired     = errors.New("idempotency key is required")
	ErrIdempotencyKeyReused       = errors.New("idempotency_key_reused_with_different_payload")
	ErrTransferAlreadyCompleted   = errors.New("transfer already completed")
)
