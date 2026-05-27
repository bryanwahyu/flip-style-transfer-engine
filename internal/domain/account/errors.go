package account

import "errors"

var (
	ErrAccountNotFound       = errors.New("account not found")
	ErrAccountNotTransactable = errors.New("account cannot transact")
)
