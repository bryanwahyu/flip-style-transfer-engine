package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/command"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/saga"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/bankmock"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/circuitbreaker"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
	pgpkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/postgres"
	redispkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/redis"
)

func TestIntegrationHappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testDB(t)
	redisClient := testRedis(t)
	ctx := context.Background()

	srcID := seedAccount(t, db, "Alice", "IDR")
	dstID := seedAccount(t, db, "Bob", "IDR")
	seedBalance(t, db, srcID, 1_000_000, "IDR")

	transferRepo := pgpkg.NewTransferRepo(db)
	accountRepo := pgpkg.NewAccountRepo(db)
	ledgerRepo := pgpkg.NewLedgerRepo(db)
	idempotencyStore := redispkg.NewIdempotencyStore(redisClient)
	log := observability.NewLogger()

	bank := bankmock.New(bankmock.ModeSuccess)
	bankGW := bankmock.NewCircuitBreakerGateway(bank, circuitbreaker.New(5, 30*time.Second))

	createCmd := command.NewCreateTransferHandler(
		transferRepo, accountRepo, idempotencyStore,
		pgpkg.NewOutboxWriter(db), pgpkg.NewEventStore(db),
	)

	transferSaga := saga.NewTransferSaga(
		transferRepo, ledgerRepo, ledgerRepo, accountRepo,
		bankGW, pgpkg.NewOutboxWriter(db), pgpkg.NewEventStore(db), log,
	)

	result, err := createCmd.Handle(ctx, command.CreateTransferCmd{
		IdempotencyKey:  "happy-path-" + newUUID(),
		SourceAccountID: srcID,
		DestAccountID:   dstID,
		Amount:          100_000,
		Currency:        money.CurrencyIDR,
	})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	transferID, _ := transfer.ParseTransferID(result.TransferID)
	if err := transferSaga.Execute(ctx, transferID); err != nil {
		t.Fatalf("saga execute: %v", err)
	}

	t.Run("transfer completed", func(t *testing.T) {
		tx, _ := transferRepo.FindByID(ctx, transferID)
		if tx.State != transfer.StateCompleted {
			t.Errorf("want COMPLETED, got %s (reason: %q)", tx.State, tx.FailureReason)
		}
	})

	t.Run("source balance decreased by transfer amount", func(t *testing.T) {
		balance, _ := ledgerRepo.GetBalance(ctx, mustAccountID(t, srcID), money.CurrencyIDR)
		if balance.Amount != 900_000 {
			t.Errorf("want 900000, got %d", balance.Amount)
		}
	})

	t.Run("dest balance increased by transfer amount", func(t *testing.T) {
		balance, _ := ledgerRepo.GetBalance(ctx, mustAccountID(t, dstID), money.CurrencyIDR)
		if balance.Amount != 100_000 {
			t.Errorf("want 100000, got %d", balance.Amount)
		}
	})

	t.Run("bank called exactly once", func(t *testing.T) {
		if bank.CallCount() != 1 {
			t.Errorf("want 1 bank call, got %d", bank.CallCount())
		}
	})
}
