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

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
	pgpkg "github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/postgres"
)

// ledgerAuditor is a local interface defining exactly what the reconciler needs.
// Following DIP: the reconciler depends on an abstraction, not a concrete type.
type ledgerAuditor interface {
	GetAllEntries(ctx context.Context) ([]ledger.LedgerEntry, error)
}

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

	db, err := pgxpool.New(ctx, envOrDefault("DATABASE_URL", "postgres://flip:flip@localhost:5432/flip?sslmode=disable"))
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer db.Close()

	auditor := pgpkg.NewLedgerRepo(db) // satisfies ledgerAuditor interface
	interval := 60 * time.Second
	log.Info("reconciler started", "interval_s", interval.Seconds())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := reconcile(ctx, auditor, log); err != nil {
				log.Error("reconciliation error", "error", err)
			}
		}
	}
}

// reconcile checks that the sum of all ledger entries equals zero (double-entry invariant).
func reconcile(ctx context.Context, auditor ledgerAuditor, log *slog.Logger) error {
	entries, err := auditor.GetAllEntries(ctx)
	if err != nil {
		return fmt.Errorf("load ledger entries: %w", err)
	}

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
