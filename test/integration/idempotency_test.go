package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/command"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/bankmock"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/circuitbreaker"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
	pgpkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/postgres"
	redispkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/redis"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/saga"
)

// TestIntegrationIdempotency verifies that retrying POST /transfers with the same
// Idempotency-Key does not create a second transfer or double-debit the source account.
func TestIntegrationIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testDB(t)
	redisClient := testRedis(t)
	ctx := context.Background()

	srcID := seedAccount(t, db, "Alice-Idempotency", "IDR")
	dstID := seedAccount(t, db, "Bob-Idempotency", "IDR")
	seedBalance(t, db, srcID, 500_000, "IDR")

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
	transferSaga := saga.NewTransferSaga(
		transferRepo, ledgerRepo, accountRepo, bankGW, outboxWriter, eventStore, log,
	)
	createCmd := command.NewCreateTransferHandler(
		transferRepo, accountRepo, idempotencyStore, outboxWriter, eventStore,
	)

	key := "idempotency-test-" + newUUID()
	cmd := command.CreateTransferCmd{
		IdempotencyKey:  key,
		SourceAccountID: srcID,
		DestAccountID:   dstID,
		Amount:          50_000,
		Currency:        ledger.CurrencyIDR,
	}

	// First call — should create the transfer.
	r1, err := createCmd.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if r1.Cached {
		t.Error("first call should not be cached")
	}

	// Run the saga to completion.
	transferID, _ := transfer.ParseTransferID(r1.TransferID)
	if err := transferSaga.Execute(ctx, transferID); err != nil {
		t.Fatalf("saga: %v", err)
	}

	// Second call — same key, same body: must return the cached response without creating a new transfer.
	r2, err := createCmd.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("retry call: %v", err)
	}
	if !r2.Cached {
		t.Error("retry should be served from cache")
	}
	if r1.TransferID != r2.TransferID {
		t.Errorf("retry returned different transfer ID: %s vs %s", r1.TransferID, r2.TransferID)
	}

	// Verify source balance was only debited once.
	balance, err := ledgerRepo.GetBalance(ctx, mustParseAccountID(t, srcID), ledger.CurrencyIDR)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	expected := int64(500_000 - 50_000)
	if balance.Amount != expected {
		t.Errorf("source balance: want %d, got %d — possible double-debit!", expected, balance.Amount)
	}

	// Bank should have been called exactly once.
	if bank.CallCount() != 1 {
		t.Errorf("bank call count: want 1, got %d", bank.CallCount())
	}
}
