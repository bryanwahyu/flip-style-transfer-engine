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
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
)

// Handler holds all HTTP dependencies and implements the route handlers.
type Handler struct {
	createTransfer *command.CreateTransferHandler
	transfers      port.TransferRepository
	accounts       port.AccountRepository
	ledgerRepo     port.LedgerRepository
	idempotency    port.IdempotencyStore
	log            *slog.Logger
}

func NewHandler(
	createTransfer *command.CreateTransferHandler,
	transfers port.TransferRepository,
	accounts port.AccountRepository,
	ledgerRepo port.LedgerRepository,
	idempotency port.IdempotencyStore,
	log *slog.Logger,
) *Handler {
	return &Handler{
		createTransfer: createTransfer,
		transfers:      transfers,
		accounts:       accounts,
		ledgerRepo:     ledgerRepo,
		idempotency:    idempotency,
		log:            log,
	}
}

// createTransferRequest is the JSON body for POST /v1/transfers.
type createTransferRequest struct {
	SourceAccountID string `json:"source_account_id"`
	DestAccountID   string `json:"dest_account_id"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	Description     string `json:"description"`
}

// CreateTransfer handles POST /v1/transfers.
func (h *Handler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	log := observability.FromContext(r.Context(), h.log)

	var req createTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", err.Error())
		return
	}

	idempotencyKey := idempotencyKeyFromCtx(r.Context())

	result, err := h.createTransfer.Handle(r.Context(), command.CreateTransferCmd{
		IdempotencyKey:  idempotencyKey,
		SourceAccountID: req.SourceAccountID,
		DestAccountID:   req.DestAccountID,
		Amount:          req.Amount,
		Currency:        ledger.Currency(req.Currency),
		Description:     req.Description,
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

// GetTransfer handles GET /v1/transfers/{id}.
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
		"id":                t.ID.String(),
		"state":             string(t.State),
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

// GetAccountBalance handles GET /v1/accounts/{id}/balance.
func (h *Handler) GetAccountBalance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	accountID, err := ledger.ParseAccountID(id)
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

	balance, err := h.ledgerRepo.GetBalance(r.Context(), accountID, acct.Currency)
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
	case errors.Is(err, transfer.ErrInsufficientFunds):
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
