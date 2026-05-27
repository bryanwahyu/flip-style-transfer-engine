package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
)

// LedgerRepo implements port.LedgerRepository.
// Balance is always computed as SUM(signed_amount) — never stored as a cached column.
type LedgerRepo struct {
	db *pgxpool.Pool
}

func NewLedgerRepo(db *pgxpool.Pool) *LedgerRepo {
	return &LedgerRepo{db: db}
}

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
			e.ID.String(),
			e.TransactionID.String(),
			e.AccountID.String(),
			string(e.Type),
			e.Amount.Amount,
			string(e.Amount.Currency),
			e.Description,
			e.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert ledger entry %s: %w", e.ID.String(), err)
		}
	}

	return tx.Commit(ctx)
}

func (r *LedgerRepo) GetBalance(ctx context.Context, accountID ledger.AccountID, currency ledger.Currency) (ledger.Money, error) {
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
		return ledger.Money{}, fmt.Errorf("get balance for %s: %w", accountID.String(), err)
	}
	return ledger.Money{Amount: balance, Currency: currency}, nil
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
			id, txID, accountID string
			entryType, currency  string
			amount              int64
			description         string
			createdAt           time.Time
		)
		if err := rows.Scan(&id, &txID, &accountID, &entryType, &amount, &currency, &description, &createdAt); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}

		entryID, err := ledger.ParseLedgerEntryID(id)
		if err != nil {
			return nil, err
		}
		transactionID, err := ledger.ParseTransactionID(txID)
		if err != nil {
			return nil, err
		}
		acctID, err := ledger.ParseAccountID(accountID)
		if err != nil {
			return nil, err
		}

		entries = append(entries, ledger.LedgerEntry{
			ID:            entryID,
			TransactionID: transactionID,
			AccountID:     acctID,
			Type:          ledger.EntryType(entryType),
			Amount:        ledger.Money{Amount: amount, Currency: ledger.Currency(currency)},
			Description:   description,
			CreatedAt:     createdAt,
		})
	}
	return entries, rows.Err()
}
