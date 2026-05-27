package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/command"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/saga"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/bankmock"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/circuitbreaker"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
	pgpkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/postgres"
	redispkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/redis"
)

// TestIntegrationSagaCompensation_BankPermanentFailure verifies that when the bank
// returns a permanent error, the saga reverses the debit and marks the transfer FAILED.
func TestIntegrationSagaCompensation_BankPermanentFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testDB(t)
	redisClient := testRedis(t)
	ctx := context.Background()

	srcID := seedAccount(t, db, "Alice-Comp", "IDR")
	dstID := seedAccount(t, db, "Bob-Comp", "IDR")
	seedBalance(t, db, srcID, 200_000, "IDR")

	transferRepo := pgpkg.NewTransferRepo(db)
	accountRepo := pgpkg.NewAccountRepo(db)
	ledgerRepo := pgpkg.NewLedgerRepo(db)
	outboxWriter := pgpkg.NewOutboxWriter(db)
	eventStore := pgpkg.NewEventStore(db)
	idempotencyStore := redispkg.NewIdempotencyStore(redisClient)
	log := observability.NewLogger()

	// Bank always returns 500 — saga must compensate.
	bank := bankmock.New(bankmock.ModePermanentErr)
	cb := circuitbreaker.New(5, 30*time.Second)
	bankGW := bankmock.NewCircuitBreakerGateway(bank, cb)

	createCmd := command.NewCreateTransferHandler(
		transferRepo, accountRepo, idempotencyStore, outboxWriter, eventStore,
	)
	transferSaga := saga.NewTransferSaga(
		transferRepo, ledgerRepo, accountRepo, bankGW, outboxWriter, eventStore, log,
	)

	r, err := createCmd.Handle(ctx, command.CreateTransferCmd{
		IdempotencyKey:  "compensation-" + newUUID(),
		SourceAccountID: srcID,
		DestAccountID:   dstID,
		Amount:          50_000,
		Currency:        ledger.CurrencyIDR,
	})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	transferID, _ := transfer.ParseTransferID(r.TransferID)

	// Execute saga — bank will fail, debit should be reversed.
	// Execute may return nil (saga handles failure internally).
	transferSaga.Execute(ctx, transferID) //nolint:errcheck

	t.Run("transfer is FAILED", func(t *testing.T) {
		tx, err := transferRepo.FindByID(ctx, transferID)
		if err != nil {
			t.Fatalf("find transfer: %v", err)
		}
		if tx.State != transfer.StateFailed {
			t.Errorf("want FAILED, got %s", tx.State)
		}
	})

	t.Run("source balance is restored after compensation", func(t *testing.T) {
		balance, err := ledgerRepo.GetBalance(ctx, mustParseAccountID(t, srcID), ledger.CurrencyIDR)
		if err != nil {
			t.Fatalf("get balance: %v", err)
		}
		if balance.Amount != 200_000 {
			t.Errorf("want restored balance 200000, got %d — compensation failed!", balance.Amount)
		}
	})

	t.Run("dest balance unchanged", func(t *testing.T) {
		balance, err := ledgerRepo.GetBalance(ctx, mustParseAccountID(t, dstID), ledger.CurrencyIDR)
		if err != nil {
			t.Fatalf("get balance: %v", err)
		}
		if balance.Amount != 0 {
			t.Errorf("want dest balance 0, got %d", balance.Amount)
		}
	})
}

// TestIntegrationSagaCompensation_InsufficientFunds verifies that a transfer with
// insufficient funds fails fast at the debit step with no ledger entries posted.
func TestIntegrationSagaCompensation_InsufficientFunds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testDB(t)
	redisClient := testRedis(t)
	ctx := context.Background()

	srcID := seedAccount(t, db, "Poor-Alice", "IDR")
	dstID := seedAccount(t, db, "Bob-Funds", "IDR")
	seedBalance(t, db, srcID, 1_000, "IDR") // only IDR 1,000

	transferRepo := pgpkg.NewTransferRepo(db)
	accountRepo := pgpkg.NewAccountRepo(db)
	ledgerRepo := pgpkg.NewLedgerRepo(db)
	outboxWriter := pgpkg.NewOutboxWriter(db)
	eventStore := pgpkg.NewEventStore(db)
	idempotencyStore := redispkg.NewIdempotencyStore(redisClient)
	log := observability.NewLogger()

	bank := bankmock.New(bankmock.ModeSuccess)
	cb := circuitbreaker.New(5, 30*time.Second)
	bankGW := bankmock.NewCircuitBreakerGateway(bank, cb)

	createCmd := command.NewCreateTransferHandler(
		transferRepo, accountRepo, idempotencyStore, outboxWriter, eventStore,
	)
	transferSaga := saga.NewTransferSaga(
		transferRepo, ledgerRepo, accountRepo, bankGW, outboxWriter, eventStore, log,
	)

	r, err := createCmd.Handle(ctx, command.CreateTransferCmd{
		IdempotencyKey:  "insufficient-" + newUUID(),
		SourceAccountID: srcID,
		DestAccountID:   dstID,
		Amount:          999_999, // way more than available
		Currency:        ledger.CurrencyIDR,
	})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	transferID, _ := transfer.ParseTransferID(r.TransferID)
	transferSaga.Execute(ctx, transferID) //nolint:errcheck

	t.Run("transfer is FAILED", func(t *testing.T) {
		tx, err := transferRepo.FindByID(ctx, transferID)
		if err != nil {
			t.Fatalf("find transfer: %v", err)
		}
		if tx.State != transfer.StateFailed {
			t.Errorf("want FAILED, got %s", tx.State)
		}
	})

	t.Run("bank was never called", func(t *testing.T) {
		if bank.CallCount() != 0 {
			t.Errorf("want 0 bank calls, got %d", bank.CallCount())
		}
	})

	t.Run("source balance unchanged", func(t *testing.T) {
		balance, err := ledgerRepo.GetBalance(ctx, mustParseAccountID(t, srcID), ledger.CurrencyIDR)
		if err != nil {
			t.Fatalf("get balance: %v", err)
		}
		if balance.Amount != 1_000 {
			t.Errorf("want 1000, got %d", balance.Amount)
		}
	})
}

// TestIntegrationCircuitBreaker verifies that the circuit breaker opens after
// repeated bank failures and blocks further calls.
func TestIntegrationCircuitBreaker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testDB(t)
	redisClient := testRedis(t)
	ctx := context.Background()

	// Circuit opens after 3 failures.
	bank := bankmock.New(bankmock.ModePermanentErr)
	cb := circuitbreaker.New(3, 30*time.Second)
	bankGW := bankmock.NewCircuitBreakerGateway(bank, cb)

	transferRepo := pgpkg.NewTransferRepo(db)
	accountRepo := pgpkg.NewAccountRepo(db)
	ledgerRepo := pgpkg.NewLedgerRepo(db)
	outboxWriter := pgpkg.NewOutboxWriter(db)
	eventStore := pgpkg.NewEventStore(db)
	idempotencyStore := redispkg.NewIdempotencyStore(redisClient)
	log := observability.NewLogger()

	transferSaga := saga.NewTransferSaga(
		transferRepo, ledgerRepo, accountRepo, bankGW, outboxWriter, eventStore, log,
	)
	createCmd := command.NewCreateTransferHandler(
		transferRepo, accountRepo, idempotencyStore, outboxWriter, eventStore,
	)

	// Trigger 3 failures to open the circuit.
	for i := 0; i < 3; i++ {
		srcID := seedAccount(t, db, "CB-Alice", "IDR")
		dstID := seedAccount(t, db, "CB-Bob", "IDR")
		seedBalance(t, db, srcID, 100_000, "IDR")

		r, _ := createCmd.Handle(ctx, command.CreateTransferCmd{
			IdempotencyKey:  newUUID(),
			SourceAccountID: srcID,
			DestAccountID:   dstID,
			Amount:          10_000,
			Currency:        ledger.CurrencyIDR,
		})
		transferID, _ := transfer.ParseTransferID(r.TransferID)
		transferSaga.Execute(ctx, transferID) //nolint:errcheck
	}

	t.Run("circuit is open after 3 failures", func(t *testing.T) {
		state := bankGW.State()
		if state != "OPEN" {
			t.Errorf("want circuit OPEN, got %s", state)
		}
	})
}
