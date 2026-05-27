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
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
	pgpkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/postgres"
	redispkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/redis"
	httphandler "github.com/bryanwahyu/flip-style-transfer-engine/internal/interfaces/http"
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

	db, err := pgxpool.New(ctx, envOrDefault("DATABASE_URL", "postgres://flip:flip@localhost:5432/flip?sslmode=disable"))
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	opts, err := goredis.ParseURL(envOrDefault("REDIS_URL", "redis://localhost:6379"))
	if err != nil {
		return fmt.Errorf("parse redis url: %w", err)
	}
	redisClient := goredis.NewClient(opts)
	defer redisClient.Close()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}

	log.Info("dependencies connected")

	transferRepo := pgpkg.NewTransferRepo(db)
	accountRepo := pgpkg.NewAccountRepo(db)
	ledgerRepo := pgpkg.NewLedgerRepo(db)
	idempotencyStore := redispkg.NewIdempotencyStore(redisClient)

	createTransferCmd := command.NewCreateTransferHandler(
		transferRepo, accountRepo, idempotencyStore,
		pgpkg.NewOutboxWriter(db), pgpkg.NewEventStore(db),
	)

	handler := httphandler.NewHandler(
		createTransferCmd,
		transferRepo,   // TransferReader
		accountRepo,    // AccountReader
		ledgerRepo,     // BalanceReader
		idempotencyStore,
		log,
	)

	srv := &http.Server{
		Addr:         ":" + envOrDefault("PORT", "8080"),
		Handler:      httphandler.NewRouter(handler),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api server started", "port", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
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
