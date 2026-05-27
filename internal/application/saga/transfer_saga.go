// Package saga contains the TransferSaga orchestrator.
//
// Design: each saga step is a named function registered in a map keyed by the
// state the transfer is in when that step should run.  Execute() is purely a
// dispatcher — it contains zero business logic, satisfying OCP: adding a new
// step only requires adding a new entry to the map, never modifying Execute().
package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/port"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

// stepFn is the signature every saga step must satisfy.
type stepFn func(ctx context.Context, t *transfer.Transfer) error

// step pairs a forward action with its compensating action.
type step struct {
	execute    stepFn
	compensate stepFn // nil if this step has no compensation (e.g. bank call)
}

// deps groups the saga's infrastructure dependencies.
type deps struct {
	transfers port.TransferRepository
	entries   port.EntryWriter
	balances  port.BalanceReader
	accounts  port.AccountLocker
	bank      port.BankGateway
	outbox    port.OutboxWriter
	events    port.TransferEventStore
}

// TransferSaga orchestrates the multi-step transfer lifecycle.
// It is stateless — the transfer's State column in the DB is the authoritative
// resume point after a crash.
type TransferSaga struct {
	deps
	log   *slog.Logger
	steps map[transfer.State]step
}

func NewTransferSaga(
	transfers port.TransferRepository,
	entries port.EntryWriter,
	balances port.BalanceReader,
	accounts port.AccountLocker,
	bank port.BankGateway,
	outbox port.OutboxWriter,
	events port.TransferEventStore,
	log *slog.Logger,
) *TransferSaga {
	s := &TransferSaga{
		deps: deps{
			transfers: transfers, entries: entries, balances: balances,
			accounts: accounts, bank: bank, outbox: outbox, events: events,
		},
		log: log,
	}

	// Step map: state → {forward, compensate}.
	// Terminal states (COMPLETED, FAILED) are absent — Execute returns nil for them.
	s.steps = map[transfer.State]step{
		transfer.StatePending:      {s.reserveDebit, s.reverseDebit},
		transfer.StateDebited:      {s.callBank, nil},
		transfer.StateBankCalled:   {s.postCredit, s.reverseCredit},
		transfer.StateCredited:     {s.complete, nil},
		transfer.StateCompensating: {s.compensate, nil},
	}

	return s
}

// Execute resumes the saga from the transfer's current state.
// Safe to call multiple times for the same transfer (idempotent).
func (s *TransferSaga) Execute(ctx context.Context, id transfer.TransferID) error {
	t, err := s.transfers.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("load transfer: %w", err)
	}

	st, ok := s.steps[t.State]
	if !ok {
		return nil // terminal state — nothing to do
	}

	return st.execute(ctx, t)
}

// ── Step implementations ──────────────────────────────────────────────────────

func (s *TransferSaga) reserveDebit(ctx context.Context, t *transfer.Transfer) error {
	log := s.log.With("transfer_id", t.ID.String(), "step", "reserve_debit")

	src, err := s.accounts.LockForUpdate(ctx, t.SourceAccountID)
	if err != nil {
		return fmt.Errorf("lock source account: %w", err)
	}
	if err := src.CanTransact(); err != nil {
		return s.failWith(ctx, t, err.Error())
	}

	balance, err := s.balances.GetBalance(ctx, t.SourceAccountID, t.Amount.Currency)
	if err != nil {
		return fmt.Errorf("get balance: %w", err)
	}
	if balance.Amount < t.Amount.Amount {
		return s.failWith(ctx, t, money.ErrInsufficientFunds.Error())
	}

	floatID := account.FloatAccounts[t.Amount.Currency]
	posting, err := ledger.NewPosting(
		ledger.NewTransactionID(), t.SourceAccountID, floatID, t.Amount,
		fmt.Sprintf("debit reserve transfer/%s", t.ID.String()),
	)
	if err != nil {
		return fmt.Errorf("build debit posting: %w", err)
	}
	if err := s.entries.PostEntries(ctx, posting.Entries()); err != nil {
		return fmt.Errorf("post debit entries: %w", err)
	}

	t.DebitPosted = true
	if err := t.Transition(transfer.StateDebited); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update state: %w", err)
	}

	log.Info("debit reserved")
	return s.publishEvent(ctx, t, transfer.EventTransferDebited)
}

func (s *TransferSaga) callBank(ctx context.Context, t *transfer.Transfer) error {
	log := s.log.With("transfer_id", t.ID.String(), "step", "call_bank")

	result, err := s.bank.InitiateTransfer(ctx, port.BankCallRequest{
		TransferID:      t.ID,
		SourceAccountID: t.SourceAccountID,
		DestAccountID:   t.DestAccountID,
		Amount:          t.Amount,
		Description:     fmt.Sprintf("transfer/%s", t.ID.String()),
	})
	if err != nil {
		log.Warn("bank call failed", "error", err)
		return s.startCompensation(ctx, t, err.Error())
	}

	t.ExternalRef = result.ExternalRef
	if err := t.Transition(transfer.StateBankCalled); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update state: %w", err)
	}

	log.Info("bank acknowledged", "external_ref", result.ExternalRef)
	return s.publishEvent(ctx, t, transfer.EventTransferBankCalled)
}

func (s *TransferSaga) postCredit(ctx context.Context, t *transfer.Transfer) error {
	log := s.log.With("transfer_id", t.ID.String(), "step", "post_credit")

	floatID := account.FloatAccounts[t.Amount.Currency]
	posting, err := ledger.NewPosting(
		ledger.NewTransactionID(), floatID, t.DestAccountID, t.Amount,
		fmt.Sprintf("credit transfer/%s", t.ID.String()),
	)
	if err != nil {
		return fmt.Errorf("build credit posting: %w", err)
	}
	if err := s.entries.PostEntries(ctx, posting.Entries()); err != nil {
		return fmt.Errorf("post credit entries: %w", err)
	}

	t.CreditPosted = true
	if err := t.Transition(transfer.StateCredited); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update state: %w", err)
	}

	log.Info("credit posted")
	return s.publishEvent(ctx, t, transfer.EventTransferCredited)
}

func (s *TransferSaga) complete(ctx context.Context, t *transfer.Transfer) error {
	if err := t.Transition(transfer.StateCompleted); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update state: %w", err)
	}
	s.log.Info("transfer completed", "transfer_id", t.ID.String())
	return s.publishEvent(ctx, t, transfer.EventTransferCompleted)
}

// ── Compensation ─────────────────────────────────────────────────────────────

func (s *TransferSaga) startCompensation(ctx context.Context, t *transfer.Transfer, reason string) error {
	t.FailureReason = reason
	if err := t.Transition(transfer.StateCompensating); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update state: %w", err)
	}
	return s.compensate(ctx, t)
}

// compensate reverses posted ledger legs in reverse order (credit first, then debit).
func (s *TransferSaga) compensate(ctx context.Context, t *transfer.Transfer) error {
	log := s.log.With("transfer_id", t.ID.String(), "step", "compensate")

	if t.CreditPosted {
		if err := s.reverseCredit(ctx, t); err != nil {
			return err
		}
	}
	if t.DebitPosted {
		if err := s.reverseDebit(ctx, t); err != nil {
			return err
		}
	}

	log.Info("compensation complete")
	return s.failWith(ctx, t, t.FailureReason)
}

func (s *TransferSaga) reverseDebit(ctx context.Context, t *transfer.Transfer) error {
	floatID := account.FloatAccounts[t.Amount.Currency]
	// Reversal: float → src (returns the reserved funds)
	posting, err := ledger.NewPosting(
		ledger.NewTransactionID(), floatID, t.SourceAccountID, t.Amount,
		fmt.Sprintf("REVERSAL debit transfer/%s", t.ID.String()),
	)
	if err != nil {
		return fmt.Errorf("build debit reversal: %w", err)
	}
	return s.entries.PostEntries(ctx, posting.Entries())
}

func (s *TransferSaga) reverseCredit(ctx context.Context, t *transfer.Transfer) error {
	floatID := account.FloatAccounts[t.Amount.Currency]
	// Reversal: dst → float (claws back the credit)
	posting, err := ledger.NewPosting(
		ledger.NewTransactionID(), t.DestAccountID, floatID, t.Amount,
		fmt.Sprintf("REVERSAL credit transfer/%s", t.ID.String()),
	)
	if err != nil {
		return fmt.Errorf("build credit reversal: %w", err)
	}
	return s.entries.PostEntries(ctx, posting.Entries())
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (s *TransferSaga) failWith(ctx context.Context, t *transfer.Transfer, reason string) error {
	t.FailureReason = reason
	if t.State == transfer.StateFailed {
		return nil // already failed — idempotent
	}
	if err := t.Transition(transfer.StateFailed); err != nil {
		return err
	}
	if err := s.transfers.UpdateState(ctx, t); err != nil {
		return fmt.Errorf("update state to failed: %w", err)
	}
	s.log.Warn("transfer failed", "transfer_id", t.ID.String(), "reason", reason)
	return s.publishEvent(ctx, t, transfer.EventTransferFailed)
}

func (s *TransferSaga) publishEvent(ctx context.Context, t *transfer.Transfer, typ transfer.EventType) error {
	evt := transfer.NewEvent(t, typ)
	if err := s.events.Append(ctx, evt); err != nil {
		return fmt.Errorf("append event: %w", err)
	}

	payload, err := json.Marshal(struct {
		TransferID string    `json:"transfer_id"`
		EventType  string    `json:"event_type"`
		State      string    `json:"state"`
		Timestamp  time.Time `json:"timestamp"`
	}{t.ID.String(), string(typ), string(t.State), time.Now().UTC()})
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}

	return s.outbox.Write(ctx, string(typ), payload)
}
