package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
)

// LedgerRepo implements port.EntryWriter, port.BalanceReader, and port.LedgerAuditor.
// Callers receive only the interface they need (ISP — see port/repositories.go).
type LedgerRepo struct{ db *pgxpool.Pool }

func NewLedgerRepo(db *pgxpool.Pool) *LedgerRepo { return &LedgerRepo{db: db} }

func (r *LedgerRepo) PostEntries(ctx context.Context, entries []ledger.LedgerEntry) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, e := range entries {
		_, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (
				id, transaction_id, account_id, entry_type,
				amount, currency, description, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			e.ID.UUID.String(),
			e.TransactionID.UUID.String(),
			e.AccountID.String(),
			string(e.Type),
			e.Amount.Amount,
			string(e.Amount.Currency),
			e.Description,
			e.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert ledger entry: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *LedgerRepo) GetBalance(ctx context.Context, accountID account.AccountID, currency money.Currency) (money.Money, error) {
	var balance int64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(
			SUM(CASE WHEN entry_type = 'CREDIT' THEN amount ELSE -amount END),
			0
		)
		FROM ledger_entries
		WHERE account_id = $1 AND currency = $2`,
		accountID.String(), string(currency),
	).Scan(&balance)
	if err != nil {
		return money.Money{}, fmt.Errorf("get balance for %s: %w", accountID, err)
	}
	return money.Money{Amount: balance, Currency: currency}, nil
}

func (r *LedgerRepo) GetAllEntries(ctx context.Context) ([]ledger.LedgerEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, transaction_id, account_id, entry_type, amount, currency, description, created_at
		FROM ledger_entries ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("query ledger entries: %w", err)
	}
	defer rows.Close()

	var entries []ledger.LedgerEntry
	for rows.Next() {
		var (
			id, txID, accountID         string
			entryType, currency         string
			amount                      int64
			description                 string
			createdAt                   time.Time
		)
		if err := rows.Scan(&id, &txID, &accountID, &entryType, &amount, &currency, &description, &createdAt); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}

		entryID, _ := ledger.ParseLedgerEntryID(id)
		transactionID, _ := ledger.ParseTransactionID(txID)
		acctID, _ := account.ParseAccountID(accountID)

		entries = append(entries, ledger.LedgerEntry{
			ID:            entryID,
			TransactionID: transactionID,
			AccountID:     acctID,
			Type:          ledger.EntryType(entryType),
			Amount:        money.Money{Amount: amount, Currency: money.Currency(currency)},
			Description:   description,
			CreatedAt:     createdAt,
		})
	}
	return entries, rows.Err()
}
