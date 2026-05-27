package port

import (
	"context"
	"errors"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

var ErrBankGatewayUnavailable = errors.New("bank gateway unavailable")

// BankCallRequest is the anti-corruption layer between our domain and external bank APIs.
type BankCallRequest struct {
	TransferID      transfer.TransferID
	SourceAccountID account.AccountID
	DestAccountID   account.AccountID
	Amount          money.Money
	Description     string
}

// BankCallResult carries the bank's acknowledgement reference.
type BankCallResult struct {
	ExternalRef string
}

// BankGateway abstracts the external bank API.
// All implementations must be safe for concurrent calls.
type BankGateway interface {
	InitiateTransfer(ctx context.Context, req BankCallRequest) (BankCallResult, error)
}
