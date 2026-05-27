package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/port"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

// TransferSaga orchestrates the multi-step transfer lifecycle.
//
// Happy path:  PENDING → ReserveDebit → DEBITED → CallBank → BANK_CALLED → PostCredit → CREDITED → COMPLETED
// Compensation: on any step failure, reverse in opposite order back to FAILED.
//
// Every step is idempotent: re-executing a step that already completed is a no-op.
type TransferSaga struct {
	transfers  port.TransferRepository
	ledger     port.LedgerRepository
	accounts   port.AccountRepository
	bank       port.BankGateway
	outbox     port.OutboxWriter
	events     port.TransferEventStore
	log        *slog.Logger
}

func NewTransferSaga(
	transfers port.TransferRepository,
	ledger port.LedgerRepository,
	accounts port.AccountRepository,
	bank port.BankGateway,
	outbox port.OutboxWriter,
	events port.TransferEventStore,
	log *slog.Logger,
) *TransferSaga {
	return &TransferSaga{
		transfers: transfers,
		ledger:    ledger,
		accounts:  accounts,
		bank:      bank,
		outbox:    outbox,
		events:    events,
		log:       log,
	}
}

// Execute resumes the saga from wherever the transfer currently sits.
// It is safe to call Execute multiple times for the same transfer (idempotent).
func (s *TransferSaga) Execute(ctx context.Context, transferID transfer.TransferID) error {
	t, err := s.transfers.FindByID(ctx, transferID)
	if err != nil {
		return fmt.Errorf("load transfer: %w", err)
	}

	log := s.log.With("transfer_id", t.ID.String(), "state", string(t.State))

	switch t.State {
	case transfer.StatePending:
		return s.reserveDebit(ctx, t, log)
	case transfer.StateDebited:
		return s.callBank(ctx, t, log)
	case transfer.StateBankCalled:
		return s.postCredit(ctx, t, log)
	case transfer.StateCredited:
		return s.completeTransfer(ctx, t, log)
	case transfer.StateCompleted, transfer.StateFailed:
		return nil // terminal states, nothing to do
	case transfer.StateCompensating:
		return s.compensate(ctx, t, log)
	default:
		return fmt.Errorf("unhandled transfer state: %s", t.State)
	}
}

// reserveDebit debits the source account and records the ledger entry atomically.
// PENDING → DEBITED
func (s *TransferSaga) reserveDebit(ctx context.Context, t *transfer.Transfer, log *slog.Logger) error {
	log.Info("saga: reserving debit")

	src, err := s.accounts.LockForUpdate(ctx, t.SourceAccountID)
	if err != nil {
		return fmt.Errorf("lock source account: %w", err)
	}
	if err := src.CanTransact(); err != nil {
		return s.fail(ctx, t, err.Error(), log)
	}

	balance, err := s.ledger.GetBalance(ctx, t.SourceAccountID, t.Amount.Currency)
	if err != nil {
		return fmt.Errorf("get balance: %w", err)
	}
	if balance.Amount < t.Amount.Amount {
		return s.fail(ctx, t, "insufficient funds", log)
	}

	// Use a clearing account (FLOAT) as the interim destination for the debit leg.
	floatAccountID := floatAccount(t.Amount.Currency)
	txID := ledger.NewTransactionID()
	posting, err := ledger.NewPosting(txID, t.SourceAccountID, floatAccountID, t.Amount,
		fmt.Sprintf("debit reserve for transfer %s", t.ID.String()))
	if err != nil {
		return fmt.Errorf("create debit posting: %w", err)
	}

	if err := s.ledger.PostEntries(ctx, posting.Entries()); err != nil {
		return fmt.Errorf("post debit entries: %w", err)
	}

	t.DebitTxID = txID
	if err := t.Transition(transfer.StateDebited); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update transfer state: %w", err)
	}

	return s.appendAndPublish(ctx, t, transfer.EventTransferDebited, log)
}

// callBank sends the transfer request to the external bank gateway.
// DEBITED → BANK_CALLED
func (s *TransferSaga) callBank(ctx context.Context, t *transfer.Transfer, log *slog.Logger) error {
	log.Info("saga: calling bank")

	result, err := s.bank.InitiateTransfer(ctx, port.BankCallRequest{
		TransferID:      t.ID,
		SourceAccountID: t.SourceAccountID,
		DestAccountID:   t.DestAccountID,
		Amount:          t.Amount,
		Description:     fmt.Sprintf("transfer %s", t.ID.String()),
	})
	if err != nil {
		log.Warn("saga: bank call failed, compensating", "error", err)
		return s.startCompensation(ctx, t, err.Error(), log)
	}

	t.ExternalRef = result.ExternalRef
	if err := t.Transition(transfer.StateBankCalled); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update transfer state: %w", err)
	}

	return s.appendAndPublish(ctx, t, transfer.EventTransferBankCalled, log)
}

// postCredit credits the destination account.
// BANK_CALLED → CREDITED
func (s *TransferSaga) postCredit(ctx context.Context, t *transfer.Transfer, log *slog.Logger) error {
	log.Info("saga: posting credit")

	floatAccountID := floatAccount(t.Amount.Currency)
	txID := ledger.NewTransactionID()
	posting, err := ledger.NewPosting(txID, floatAccountID, t.DestAccountID, t.Amount,
		fmt.Sprintf("credit for transfer %s", t.ID.String()))
	if err != nil {
		return fmt.Errorf("create credit posting: %w", err)
	}

	if err := s.ledger.PostEntries(ctx, posting.Entries()); err != nil {
		return fmt.Errorf("post credit entries: %w", err)
	}

	t.CreditTxID = txID
	if err := t.Transition(transfer.StateCredited); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update transfer state: %w", err)
	}

	return s.appendAndPublish(ctx, t, transfer.EventTransferCredited, log)
}

// completeTransfer marks the transfer as done.
// CREDITED → COMPLETED
func (s *TransferSaga) completeTransfer(ctx context.Context, t *transfer.Transfer, log *slog.Logger) error {
	log.Info("saga: completing transfer")

	if err := t.Transition(transfer.StateCompleted); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update transfer state: %w", err)
	}

	return s.appendAndPublish(ctx, t, transfer.EventTransferCompleted, log)
}

// startCompensation transitions to COMPENSATING and kicks off reversal.
func (s *TransferSaga) startCompensation(ctx context.Context, t *transfer.Transfer, reason string, log *slog.Logger) error {
	t.FailureReason = reason
	if err := t.Transition(transfer.StateCompensating); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update transfer state: %w", err)
	}
	return s.compensate(ctx, t, log)
}

// compensate reverses any ledger entries that were posted before the failure.
func (s *TransferSaga) compensate(ctx context.Context, t *transfer.Transfer, log *slog.Logger) error {
	log.Info("saga: compensating", "reason", t.FailureReason)

	// Reverse credit leg if it was posted.
	zeroTx := ledger.TransactionID{}
	if t.CreditTxID != zeroTx {
		floatAccountID := floatAccount(t.Amount.Currency)
		reversalTxID := ledger.NewTransactionID()
		// Reverse: dst → float (undo the credit)
		posting, err := ledger.NewPosting(reversalTxID, t.DestAccountID, floatAccountID, t.Amount,
			fmt.Sprintf("REVERSAL credit for transfer %s", t.ID.String()))
		if err != nil {
			return fmt.Errorf("create credit reversal: %w", err)
		}
		if err := s.ledger.PostEntries(ctx, posting.Entries()); err != nil {
			return fmt.Errorf("post credit reversal: %w", err)
		}
	}

	// Reverse debit leg if it was posted.
	if t.DebitTxID != zeroTx {
		floatAccountID := floatAccount(t.Amount.Currency)
		reversalTxID := ledger.NewTransactionID()
		// Reverse: float → src (undo the debit)
		posting, err := ledger.NewPosting(reversalTxID, floatAccountID, t.SourceAccountID, t.Amount,
			fmt.Sprintf("REVERSAL debit for transfer %s", t.ID.String()))
		if err != nil {
			return fmt.Errorf("create debit reversal: %w", err)
		}
		if err := s.ledger.PostEntries(ctx, posting.Entries()); err != nil {
			return fmt.Errorf("post debit reversal: %w", err)
		}
	}

	return s.fail(ctx, t, t.FailureReason, log)
}

func (s *TransferSaga) fail(ctx context.Context, t *transfer.Transfer, reason string, log *slog.Logger) error {
	t.FailureReason = reason
	if err := t.Transition(transfer.StateFailed); err != nil {
		if t.State == transfer.StateFailed {
			return nil // already failed, idempotent
		}
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update transfer to failed: %w", err)
	}
	log.Warn("saga: transfer failed", "reason", reason)
	return s.appendAndPublish(ctx, t, transfer.EventTransferFailed, log)
}

func (s *TransferSaga) appendAndPublish(ctx context.Context, t *transfer.Transfer, typ transfer.EventType, log *slog.Logger) error {
	evt := transfer.NewEvent(t, typ)
	if err := s.events.Append(ctx, evt); err != nil {
		return fmt.Errorf("append event: %w", err)
	}

	payload, err := json.Marshal(sagaMessage{
		TransferID: t.ID.String(),
		EventType:  string(typ),
		State:      string(t.State),
		Timestamp:  time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("marshal saga message: %w", err)
	}
	if err := s.outbox.Write(ctx, string(typ), payload); err != nil {
		return fmt.Errorf("write outbox: %w", err)
	}

	log.Info("saga: event published", "event_type", string(typ))
	return nil
}

type sagaMessage struct {
	TransferID string    `json:"transfer_id"`
	EventType  string    `json:"event_type"`
	State      string    `json:"state"`
	Timestamp  time.Time `json:"timestamp"`
}

// floatAccount returns the system float/clearing account for a given currency.
// In production this would be looked up from configuration or the database.
func floatAccount(currency ledger.Currency) ledger.AccountID {
	// Deterministic UUID derived from currency — always the same float account per currency.
	id, _ := ledger.ParseAccountID(fmt.Sprintf("00000000-0000-0000-%04s-000000000000",
		fmt.Sprintf("%-4s", string(currency))))
	return id
}
