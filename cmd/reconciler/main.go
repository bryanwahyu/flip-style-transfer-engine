package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
	pgpkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/postgres"
)

func main() {
	log := observability.NewLogger()
	if err := run(log); err != nil {
		log.Error("reconciler fatal error", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbURL := envOrDefault("DATABASE_URL", "postgres://flip:flip@localhost:5432/flip?sslmode=disable")
	interval := 60 * time.Second

	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	ledgerRepo := pgpkg.NewLedgerRepo(db)

	log.Info("reconciler started", "interval_s", interval.Seconds())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := reconcile(ctx, ledgerRepo, log); err != nil {
				log.Error("reconciliation error", "error", err)
			}
		}
	}
}

// reconcile checks that the sum of all ledger entries equals zero (double-entry invariant).
// Any drift indicates a bug in posting logic and is flagged as a critical alert.
func reconcile(ctx context.Context, ledger *pgpkg.LedgerRepo, log *slog.Logger) error {
	entries, err := ledger.GetAllEntries(ctx)
	if err != nil {
		return fmt.Errorf("load ledger entries: %w", err)
	}

	// Group signed balances by currency.
	totals := make(map[string]int64)
	for _, e := range entries {
		totals[string(e.Amount.Currency)] += e.SignedAmount()
	}

	drift := false
	for currency, total := range totals {
		if total != 0 {
			log.Error("LEDGER DRIFT DETECTED",
				"currency", currency,
				"signed_total", total,
				"action", "accounts flagged for manual review",
			)
			drift = true
		}
	}

	if !drift {
		log.Info("reconciliation OK", "entries_checked", len(entries))
	}

	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
