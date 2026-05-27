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

// IntegrationTestHappyPath verifies the complete SAGA happy path end to end.
// Requires: docker compose up (postgres + redis + nats).
func TestIntegrationHappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testDB(t)
	redisClient := testRedis(t)
	ctx := context.Background()

	// Seed two accounts with an initial balance.
	srcID := seedAccount(t, db, "Alice", "IDR")
	dstID := seedAccount(t, db, "Bob", "IDR")
	seedBalance(t, db, srcID, 1_000_000, "IDR") // IDR 1,000,000 initial balance

	// Wire up repositories and saga.
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

	// 1. Create transfer.
	result, err := createCmd.Handle(ctx, command.CreateTransferCmd{
		IdempotencyKey:  "test-happy-path-" + newUUID(),
		SourceAccountID: srcID,
		DestAccountID:   dstID,
		Amount:          100_000,
		Currency:        ledger.CurrencyIDR,
	})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}
	if result.State != transfer.StatePending {
		t.Fatalf("expected PENDING, got %s", result.State)
	}

	transferID, err := transfer.ParseTransferID(result.TransferID)
	if err != nil {
		t.Fatalf("parse transfer id: %v", err)
	}

	// 2. Run the full saga (synchronously in test).
	if err := transferSaga.Execute(ctx, transferID); err != nil {
		t.Fatalf("saga execute: %v", err)
	}

	// 3. Verify final state.
	t.Run("transfer is COMPLETED", func(t *testing.T) {
		tx, err := transferRepo.FindByID(ctx, transferID)
		if err != nil {
			t.Fatalf("find transfer: %v", err)
		}
		if tx.State != transfer.StateCompleted {
			t.Errorf("want COMPLETED, got %s (failure_reason=%q)", tx.State, tx.FailureReason)
		}
	})

	t.Run("source balance decreased", func(t *testing.T) {
		balance, err := ledgerRepo.GetBalance(ctx, mustParseAccountID(t, srcID), ledger.CurrencyIDR)
		if err != nil {
			t.Fatalf("get balance: %v", err)
		}
		// Initial: 1,000,000 minus 100,000 = 900,000
		if balance.Amount != 900_000 {
			t.Errorf("want 900000, got %d", balance.Amount)
		}
	})

	t.Run("dest balance increased", func(t *testing.T) {
		balance, err := ledgerRepo.GetBalance(ctx, mustParseAccountID(t, dstID), ledger.CurrencyIDR)
		if err != nil {
			t.Fatalf("get balance: %v", err)
		}
		if balance.Amount != 100_000 {
			t.Errorf("want 100000, got %d", balance.Amount)
		}
	})

	t.Run("bank was called once", func(t *testing.T) {
		if bank.CallCount() != 1 {
			t.Errorf("want 1 bank call, got %d", bank.CallCount())
		}
	})
}

func mustParseAccountID(t *testing.T, s string) ledger.AccountID {
	t.Helper()
	id, err := ledger.ParseAccountID(s)
	if err != nil {
		t.Fatalf("parse account id %q: %v", s, err)
	}
	return id
}
