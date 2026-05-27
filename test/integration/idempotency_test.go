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

func TestIntegrationIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testDB(t)
	redisClient := testRedis(t)
	ctx := context.Background()

	srcID := seedAccount(t, db, "Alice-Idem", "IDR")
	dstID := seedAccount(t, db, "Bob-Idem", "IDR")
	seedBalance(t, db, srcID, 500_000, "IDR")

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

	key := "idempotency-" + newUUID()
	cmd := command.CreateTransferCmd{
		IdempotencyKey:  key,
		SourceAccountID: srcID,
		DestAccountID:   dstID,
		Amount:          50_000,
		Currency:        money.CurrencyIDR,
	}

	r1, err := createCmd.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if r1.Cached {
		t.Error("first call should not be cached")
	}

	transferID, _ := transfer.ParseTransferID(r1.TransferID)
	if err := transferSaga.Execute(ctx, transferID); err != nil {
		t.Fatalf("saga: %v", err)
	}

	// Retry with same key — must return cached response, no second debit.
	r2, err := createCmd.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("retry call: %v", err)
	}
	if !r2.Cached {
		t.Error("retry should be served from idempotency cache")
	}
	if r1.TransferID != r2.TransferID {
		t.Errorf("retry returned different transfer ID: %s vs %s", r1.TransferID, r2.TransferID)
	}

	balance, _ := ledgerRepo.GetBalance(ctx, mustAccountID(t, srcID), money.CurrencyIDR)
	if balance.Amount != 450_000 {
		t.Errorf("source balance: want 450000 (debited once), got %d — possible double-debit!", balance.Amount)
	}

	if bank.CallCount() != 1 {
		t.Errorf("bank call count: want 1, got %d", bank.CallCount())
	}
}
