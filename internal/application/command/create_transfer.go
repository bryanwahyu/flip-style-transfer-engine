package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/port"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

// CreateTransferCmd carries the input for initiating a new transfer.
type CreateTransferCmd struct {
	IdempotencyKey  string
	SourceAccountID string
	DestAccountID   string
	Amount          int64
	Currency        ledger.Currency
	Description     string
}

// CreateTransferResult is returned after successful transfer creation.
type CreateTransferResult struct {
	TransferID string
	State      transfer.State
	Cached     bool // true if the response was served from idempotency cache
}

// CreateTransferHandler validates, deduplicates, and persists a new Transfer.
// It does NOT execute the SAGA — that happens asynchronously via the worker.
type CreateTransferHandler struct {
	transfers   port.TransferRepository
	accounts    port.AccountRepository
	idempotency port.IdempotencyStore
	outbox      port.OutboxWriter
	events      port.TransferEventStore
}

func NewCreateTransferHandler(
	transfers port.TransferRepository,
	accounts port.AccountRepository,
	idempotency port.IdempotencyStore,
	outbox port.OutboxWriter,
	events port.TransferEventStore,
) *CreateTransferHandler {
	return &CreateTransferHandler{
		transfers:   transfers,
		accounts:    accounts,
		idempotency: idempotency,
		outbox:      outbox,
		events:      events,
	}
}

func (h *CreateTransferHandler) Handle(ctx context.Context, cmd CreateTransferCmd) (CreateTransferResult, error) {
	const idempotencyTTL = 24 * time.Hour

	// Check idempotency cache first — return the original response if present.
	cached, found, err := h.idempotency.Get(ctx, cmd.IdempotencyKey)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("idempotency store get: %w", err)
	}
	if found {
		var result CreateTransferResult
		if err := json.Unmarshal(cached, &result); err != nil {
			return CreateTransferResult{}, fmt.Errorf("unmarshal cached response: %w", err)
		}
		result.Cached = true
		return result, nil
	}

	srcID, err := ledger.ParseAccountID(cmd.SourceAccountID)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("invalid source account: %w", err)
	}
	dstID, err := ledger.ParseAccountID(cmd.DestAccountID)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("invalid destination account: %w", err)
	}

	// Verify accounts exist and can transact before creating the transfer.
	src, err := h.accounts.FindByID(ctx, srcID)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("source account: %w", err)
	}
	if err := src.CanTransact(); err != nil {
		return CreateTransferResult{}, err
	}
	dst, err := h.accounts.FindByID(ctx, dstID)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("destination account: %w", err)
	}
	if err := dst.CanTransact(); err != nil {
		return CreateTransferResult{}, err
	}

	amount, err := ledger.NewMoney(cmd.Amount, cmd.Currency)
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

	evtPayload, err := json.Marshal(evt)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("marshal event: %w", err)
	}
	if err := h.outbox.Write(ctx, "transfer.requested", evtPayload); err != nil {
		return CreateTransferResult{}, fmt.Errorf("write outbox: %w", err)
	}

	result := CreateTransferResult{
		TransferID: t.ID.String(),
		State:      t.State,
	}

	// Cache the response so retries with the same key return an identical response.
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return CreateTransferResult{}, fmt.Errorf("marshal result: %w", err)
	}
	if _, err := h.idempotency.SetIfAbsent(ctx, cmd.IdempotencyKey, resultBytes, idempotencyTTL); err != nil {
		return CreateTransferResult{}, fmt.Errorf("cache result: %w", err)
	}

	return result, nil
}
