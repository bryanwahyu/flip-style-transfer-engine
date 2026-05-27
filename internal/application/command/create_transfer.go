package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/port"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

const idempotencyTTL = 24 * time.Hour

// CreateTransferCmd is the input for initiating a new transfer.
type CreateTransferCmd struct {
	IdempotencyKey  string
	SourceAccountID string
	DestAccountID   string
	Amount          int64
	Currency        money.Currency
}

// CreateTransferResult is the output after successful creation.
type CreateTransferResult struct {
	TransferID string         `json:"transfer_id"`
	State      transfer.State `json:"state"`
	Cached     bool           `json:"cached,omitempty"`
}

// CreateTransferHandler validates, deduplicates, and persists a new Transfer.
// SAGA execution is deferred to the worker — this handler only creates PENDING state.
type CreateTransferHandler struct {
	transfers   port.TransferWriter
	accounts    port.AccountReader
	idempotency port.IdempotencyStore
	outbox      port.OutboxWriter
	events      port.TransferEventStore
}

func NewCreateTransferHandler(
	transfers port.TransferWriter,
	accounts port.AccountReader,
	idempotency port.IdempotencyStore,
	outbox port.OutboxWriter,
	events port.TransferEventStore,
) *CreateTransferHandler {
	return &CreateTransferHandler{
		transfers: transfers, accounts: accounts,
		idempotency: idempotency, outbox: outbox, events: events,
	}
}

func (h *CreateTransferHandler) Handle(ctx context.Context, cmd CreateTransferCmd) (CreateTransferResult, error) {
	// Idempotency check first — cheapest operation, avoids all DB work on retries.
	if result, ok, err := h.checkIdempotency(ctx, cmd.IdempotencyKey); err != nil {
		return CreateTransferResult{}, err
	} else if ok {
		return result, nil
	}

	srcID, err := account.ParseAccountID(cmd.SourceAccountID)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("invalid source account: %w", err)
	}
	dstID, err := account.ParseAccountID(cmd.DestAccountID)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("invalid destination account: %w", err)
	}

	if err := h.validateAccounts(ctx, srcID, dstID); err != nil {
		return CreateTransferResult{}, err
	}

	amount, err := money.New(cmd.Amount, cmd.Currency)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("invalid amount: %w", err)
	}

	t, err := transfer.New(transfer.NewTransferID(), cmd.IdempotencyKey, srcID, dstID, amount)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("create transfer: %w", err)
	}

	if err := h.transfers.Save(ctx, t); err != nil {
		return CreateTransferResult{}, fmt.Errorf("save transfer: %w", err)
	}

	evt := transfer.NewEvent(t, transfer.EventTransferRequested)
	if err := h.events.Append(ctx, evt); err != nil {
		return CreateTransferResult{}, fmt.Errorf("append event: %w", err)
	}

	evtPayload, _ := json.Marshal(evt)
	if err := h.outbox.Write(ctx, string(transfer.EventTransferRequested), evtPayload); err != nil {
		return CreateTransferResult{}, fmt.Errorf("write outbox: %w", err)
	}

	result := CreateTransferResult{TransferID: t.ID.String(), State: t.State}
	return result, h.cacheResult(ctx, cmd.IdempotencyKey, result)
}

func (h *CreateTransferHandler) checkIdempotency(ctx context.Context, key string) (CreateTransferResult, bool, error) {
	cached, found, err := h.idempotency.Get(ctx, key)
	if err != nil {
		return CreateTransferResult{}, false, fmt.Errorf("idempotency get: %w", err)
	}
	if !found {
		return CreateTransferResult{}, false, nil
	}
	var result CreateTransferResult
	if err := json.Unmarshal(cached, &result); err != nil {
		return CreateTransferResult{}, false, fmt.Errorf("unmarshal cached response: %w", err)
	}
	result.Cached = true
	return result, true, nil
}

func (h *CreateTransferHandler) validateAccounts(ctx context.Context, src, dst account.AccountID) error {
	srcAcct, err := h.accounts.FindByID(ctx, src)
	if err != nil {
		return fmt.Errorf("source account: %w", err)
	}
	if err := srcAcct.CanTransact(); err != nil {
		return err
	}
	dstAcct, err := h.accounts.FindByID(ctx, dst)
	if err != nil {
		return fmt.Errorf("destination account: %w", err)
	}
	return dstAcct.CanTransact()
}

func (h *CreateTransferHandler) cacheResult(ctx context.Context, key string, result CreateTransferResult) error {
	b, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	_, err = h.idempotency.SetIfAbsent(ctx, key, b, idempotencyTTL)
	return err
}
