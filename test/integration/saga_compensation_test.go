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
	idempotencyStore := redispkg.NewIdempotencyStore(redisClient)
	log := observability.NewLogger()

	bank := bankmock.New(bankmock.ModePermanentErr) // bank always fails
	bankGW := bankmock.NewCircuitBreakerGateway(bank, circuitbreaker.New(5, 30*time.Second))

	createCmd := command.NewCreateTransferHandler(
		transferRepo, accountRepo, idempotencyStore,
		pgpkg.NewOutboxWriter(db), pgpkg.NewEventStore(db),
	)
	transferSaga := saga.NewTransferSaga(
		transferRepo, ledgerRepo, ledgerRepo, accountRepo,
		bankGW, pgpkg.NewOutboxWriter(db), pgpkg.NewEventStore(db), log,
	)

	r, _ := createCmd.Handle(ctx, command.CreateTransferCmd{
		IdempotencyKey: "comp-" + newUUID(),
		SourceAccountID: srcID, DestAccountID: dstID,
		Amount: 50_000, Currency: money.CurrencyIDR,
	})

	transferID, _ := transfer.ParseTransferID(r.TransferID)
	transferSaga.Execute(ctx, transferID) //nolint:errcheck

	t.Run("transfer is FAILED", func(t *testing.T) {
		tx, _ := transferRepo.FindByID(ctx, transferID)
		if tx.State != transfer.StateFailed {
			t.Errorf("want FAILED, got %s", tx.State)
		}
	})

	t.Run("source balance fully restored after compensation", func(t *testing.T) {
		balance, _ := ledgerRepo.GetBalance(ctx, mustAccountID(t, srcID), money.CurrencyIDR)
		if balance.Amount != 200_000 {
			t.Errorf("want 200000, got %d — compensation failed!", balance.Amount)
		}
	})

	t.Run("dest balance unchanged", func(t *testing.T) {
		balance, _ := ledgerRepo.GetBalance(ctx, mustAccountID(t, dstID), money.CurrencyIDR)
		if balance.Amount != 0 {
			t.Errorf("want 0, got %d", balance.Amount)
		}
	})
}

func TestIntegrationSagaCompensation_InsufficientFunds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testDB(t)
	redisClient := testRedis(t)
	ctx := context.Background()

	srcID := seedAccount(t, db, "Poor-Alice", "IDR")
	dstID := seedAccount(t, db, "Rich-Bob", "IDR")
	seedBalance(t, db, srcID, 1_000, "IDR")

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

	r, _ := createCmd.Handle(ctx, command.CreateTransferCmd{
		IdempotencyKey: "insuf-" + newUUID(),
		SourceAccountID: srcID, DestAccountID: dstID,
		Amount: 999_999, Currency: money.CurrencyIDR,
	})

	transferID, _ := transfer.ParseTransferID(r.TransferID)
	transferSaga.Execute(ctx, transferID) //nolint:errcheck

	t.Run("transfer is FAILED", func(t *testing.T) {
		tx, _ := transferRepo.FindByID(ctx, transferID)
		if tx.State != transfer.StateFailed {
			t.Errorf("want FAILED, got %s", tx.State)
		}
	})

	t.Run("bank never called", func(t *testing.T) {
		if bank.CallCount() != 0 {
			t.Errorf("want 0 bank calls, got %d", bank.CallCount())
		}
	})

	t.Run("source balance unchanged", func(t *testing.T) {
		balance, _ := ledgerRepo.GetBalance(ctx, mustAccountID(t, srcID), money.CurrencyIDR)
		if balance.Amount != 1_000 {
			t.Errorf("want 1000, got %d", balance.Amount)
		}
	})
}

func TestIntegrationCircuitBreaker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testDB(t)
	redisClient := testRedis(t)
	ctx := context.Background()

	bank := bankmock.New(bankmock.ModePermanentErr)
	cb := circuitbreaker.New(3, 30*time.Second)
	bankGW := bankmock.NewCircuitBreakerGateway(bank, cb)

	transferRepo := pgpkg.NewTransferRepo(db)
	accountRepo := pgpkg.NewAccountRepo(db)
	ledgerRepo := pgpkg.NewLedgerRepo(db)
	idempotencyStore := redispkg.NewIdempotencyStore(redisClient)
	log := observability.NewLogger()

	transferSaga := saga.NewTransferSaga(
		transferRepo, ledgerRepo, ledgerRepo, accountRepo,
		bankGW, pgpkg.NewOutboxWriter(db), pgpkg.NewEventStore(db), log,
	)
	createCmd := command.NewCreateTransferHandler(
		transferRepo, accountRepo, idempotencyStore,
		pgpkg.NewOutboxWriter(db), pgpkg.NewEventStore(db),
	)

	for range 3 {
		srcID := seedAccount(t, db, "CB-Alice", "IDR")
		dstID := seedAccount(t, db, "CB-Bob", "IDR")
		seedBalance(t, db, srcID, 100_000, "IDR")

		r, _ := createCmd.Handle(ctx, command.CreateTransferCmd{
			IdempotencyKey: newUUID(),
			SourceAccountID: srcID, DestAccountID: dstID,
			Amount: 10_000, Currency: money.CurrencyIDR,
		})
		id, _ := transfer.ParseTransferID(r.TransferID)
		transferSaga.Execute(ctx, id) //nolint:errcheck
	}

	t.Run("circuit is OPEN after 3 consecutive failures", func(t *testing.T) {
		if state := bankGW.State(); state != "OPEN" {
			t.Errorf("want OPEN, got %s", state)
		}
	})
}
