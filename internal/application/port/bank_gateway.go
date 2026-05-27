package port

import (
	"context"
	"errors"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

var ErrBankGatewayUnavailable = errors.New("bank gateway unavailable")

// BankCallRequest carries everything needed to initiate an interbank transfer.
type BankCallRequest struct {
	TransferID      transfer.TransferID
	SourceAccountID ledger.AccountID
	DestAccountID   ledger.AccountID
	Amount          ledger.Money
	Description     string
}

// BankCallResult carries the bank's acknowledgement reference.
type BankCallResult struct {
	ExternalRef string
}

// BankGateway is the anti-corruption layer between our domain and external bank APIs.
// All implementations must be safe for concurrent calls.
type BankGateway interface {
	InitiateTransfer(ctx context.Context, req BankCallRequest) (BankCallResult, error)
}
