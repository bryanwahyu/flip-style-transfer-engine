package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/saga"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/bankmock"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/circuitbreaker"
	natspkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/nats"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
	pgpkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/postgres"
)

func main() {
	log := observability.NewLogger()
	if err := run(log); err != nil {
		log.Error("worker fatal error", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, envOrDefault("DATABASE_URL", "postgres://flip:flip@localhost:5432/flip?sslmode=disable"))
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer db.Close()

	nc, err := nats.Connect(envOrDefault("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	defer nc.Drain() //nolint:errcheck

	transferRepo := pgpkg.NewTransferRepo(db)
	ledgerRepo := pgpkg.NewLedgerRepo(db)
	accountRepo := pgpkg.NewAccountRepo(db)

	bank := bankmock.New(bankmock.ModeSuccess)
	cb := circuitbreaker.New(5, 30*time.Second)
	bankGW := bankmock.NewCircuitBreakerGateway(bank, cb)

	transferSaga := saga.NewTransferSaga(
		transferRepo,
		ledgerRepo, // EntryWriter
		ledgerRepo, // BalanceReader
		accountRepo,
		bankGW,
		pgpkg.NewOutboxWriter(db),
		pgpkg.NewEventStore(db),
		log,
	)

	consumer, err := natspkg.NewConsumer(nc, "TRANSFERS", "worker", log)
	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}

	subjects := []string{
		"transfer.requested", "transfer.debited",
		"transfer.bank_called", "transfer.credited",
	}
	if err := consumer.EnsureStream(ctx, subjects); err != nil {
		return fmt.Errorf("ensure stream: %w", err)
	}

	log.Info("worker started")

	return consumer.Subscribe(ctx, func(ctx context.Context, _ string, data []byte) error {
		var msg struct {
			TransferID string `json:"transfer_id"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("unmarshal message: %w", err)
		}
		id, err := transfer.ParseTransferID(msg.TransferID)
		if err != nil {
			return fmt.Errorf("parse transfer id: %w", err)
		}
		return transferSaga.Execute(ctx, id)
	})
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
