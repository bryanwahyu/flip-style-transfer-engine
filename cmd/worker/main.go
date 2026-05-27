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

	dbURL := envOrDefault("DATABASE_URL", "postgres://flip:flip@localhost:5432/flip?sslmode=disable")
	natsURL := envOrDefault("NATS_URL", "nats://localhost:4222")

	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("connect to nats: %w", err)
	}
	defer nc.Drain() //nolint:errcheck

	transferRepo := pgpkg.NewTransferRepo(db)
	ledgerRepo := pgpkg.NewLedgerRepo(db)
	accountRepo := pgpkg.NewAccountRepo(db)
	outboxWriter := pgpkg.NewOutboxWriter(db)
	eventStore := pgpkg.NewEventStore(db)

	// Bank gateway with circuit breaker: open after 5 consecutive failures, reset after 30s.
	mockBank := bankmock.New(bankmock.ModeSuccess)
	cb := circuitbreaker.New(5, 30*time.Second)
	bankGateway := bankmock.NewCircuitBreakerGateway(mockBank, cb)

	transferSaga := saga.NewTransferSaga(
		transferRepo, ledgerRepo, accountRepo,
		bankGateway, outboxWriter, eventStore, log,
	)

	consumer, err := natspkg.NewConsumer(nc, "TRANSFERS", "worker", log)
	if err != nil {
		return fmt.Errorf("create nats consumer: %w", err)
	}

	subjects := []string{
		"transfer.requested",
		"transfer.debited",
		"transfer.bank_called",
		"transfer.credited",
	}
	if err := consumer.EnsureStream(ctx, subjects); err != nil {
		return fmt.Errorf("ensure stream: %w", err)
	}

	log.Info("worker started, subscribing to transfer events")

	return consumer.Subscribe(ctx, func(ctx context.Context, subject string, data []byte) error {
		var msg struct {
			TransferID string `json:"transfer_id"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("unmarshal message: %w", err)
		}

		transferID, err := transfer.ParseTransferID(msg.TransferID)
		if err != nil {
			return fmt.Errorf("parse transfer id: %w", err)
		}

		log := log.With("transfer_id", transferID.String(), "subject", subject)
		log.Info("worker: processing saga step")

		if err := transferSaga.Execute(ctx, transferID); err != nil {
			log.Error("worker: saga execution failed", "error", err)
			return err
		}
		return nil
	})
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
