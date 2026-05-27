package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/command"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/port"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
)

// Handler holds HTTP dependencies and implements the route handlers.
type Handler struct {
	createTransfer *command.CreateTransferHandler
	transfers      port.TransferReader
	accounts       port.AccountReader
	balances       port.BalanceReader
	idempotency    port.IdempotencyStore
	log            *slog.Logger
}

func NewHandler(
	createTransfer *command.CreateTransferHandler,
	transfers port.TransferReader,
	accounts port.AccountReader,
	balances port.BalanceReader,
	idempotency port.IdempotencyStore,
	log *slog.Logger,
) *Handler {
	return &Handler{
		createTransfer: createTransfer,
		transfers:      transfers,
		accounts:       accounts,
		balances:       balances,
		idempotency:    idempotency,
		log:            log,
	}
}

type createTransferRequest struct {
	SourceAccountID string `json:"source_account_id"`
	DestAccountID   string `json:"dest_account_id"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
}

func (h *Handler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	log := observability.FromContext(r.Context(), h.log)

	var req createTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", err.Error())
		return
	}

	result, err := h.createTransfer.Handle(r.Context(), command.CreateTransferCmd{
		IdempotencyKey:  idempotencyKeyFromCtx(r.Context()),
		SourceAccountID: req.SourceAccountID,
		DestAccountID:   req.DestAccountID,
		Amount:          req.Amount,
		Currency:        money.Currency(req.Currency),
	})
	if err != nil {
		log.Error("create transfer failed", "error", err)
		h.handleDomainError(w, err)
		return
	}

	status := http.StatusAccepted
	if result.Cached {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (h *Handler) GetTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	transferID, err := transfer.ParseTransferID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_transfer_id", err.Error())
		return
	}
	t, err := h.transfers.FindByID(r.Context(), transferID)
	if errors.Is(err, transfer.ErrTransferNotFound) {
		writeError(w, http.StatusNotFound, "transfer_not_found", "transfer not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": t.ID.String(), "state": string(t.State),
		"source_account_id": t.SourceAccountID.String(),
		"dest_account_id":   t.DestAccountID.String(),
		"amount":            t.Amount.Amount,
		"currency":          string(t.Amount.Currency),
		"external_ref":      t.ExternalRef,
		"failure_reason":    t.FailureReason,
		"created_at":        t.CreatedAt,
		"updated_at":        t.UpdatedAt,
	})
}

func (h *Handler) GetAccountBalance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	accountID, err := account.ParseAccountID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account_id", err.Error())
		return
	}
	acct, err := h.accounts.FindByID(r.Context(), accountID)
	if errors.Is(err, account.ErrAccountNotFound) {
		writeError(w, http.StatusNotFound, "account_not_found", "account not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	balance, err := h.balances.GetBalance(r.Context(), accountID, acct.Currency)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"account_id": id,
		"balance":    balance.Amount,
		"currency":   string(balance.Currency),
	})
}

func (h *Handler) handleDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, money.ErrInsufficientFunds):
		writeError(w, http.StatusUnprocessableEntity, "insufficient_funds", err.Error())
	case errors.Is(err, transfer.ErrIdempotencyKeyReused):
		writeError(w, http.StatusUnprocessableEntity, "idempotency_key_reused_with_different_payload", err.Error())
	case errors.Is(err, account.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account_not_found", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
