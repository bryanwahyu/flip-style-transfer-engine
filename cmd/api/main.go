package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/command"
	natspkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/nats"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
	pgpkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/postgres"
	redispkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/redis"
	httphandler "github.com/bryanwahyu/flip-style-transfer-engine/internal/interfaces/http"

	"github.com/nats-io/nats.go"
)

func main() {
	log := observability.NewLogger()

	if err := run(log); err != nil {
		log.Error("api server fatal error", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbURL := envOrDefault("DATABASE_URL", "postgres://flip:flip@localhost:5432/flip?sslmode=disable")
	redisURL := envOrDefault("REDIS_URL", "redis://localhost:6379")
	natsURL := envOrDefault("NATS_URL", "nats://localhost:4222")
	port := envOrDefault("PORT", "8080")

	// PostgreSQL
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	log.Info("connected to postgres")

	// Redis
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("parse redis url: %w", err)
	}
	redisClient := goredis.NewClient(opts)
	defer redisClient.Close()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	log.Info("connected to redis")

	// NATS (for outbox writer only — API writes to outbox table, not directly to NATS)
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("connect to nats: %w", err)
	}
	defer nc.Drain() //nolint:errcheck
	log.Info("connected to nats")
	_ = nc // outbox relay handles actual publishing

	// Repositories & stores
	transferRepo := pgpkg.NewTransferRepo(db)
	accountRepo := pgpkg.NewAccountRepo(db)
	ledgerRepo := pgpkg.NewLedgerRepo(db)
	outboxWriter := pgpkg.NewOutboxWriter(db)
	eventStore := pgpkg.NewEventStore(db)
	idempotencyStore := redispkg.NewIdempotencyStore(redisClient)

	// Application layer
	createTransferCmd := command.NewCreateTransferHandler(
		transferRepo, accountRepo, idempotencyStore, outboxWriter, eventStore,
	)

	// HTTP
	handler := httphandler.NewHandler(
		createTransferCmd, transferRepo, accountRepo, ledgerRepo, idempotencyStore, log,
	)
	router := httphandler.NewRouter(handler)

	// NATS publisher is not used directly in API — outbox relay handles that.
	_ = natspkg.NewPublisher

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api server starting", "port", port)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down api server")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
