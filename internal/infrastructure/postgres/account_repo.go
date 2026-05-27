package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/account"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
)

// AccountRepo implements port.AccountRepository.
type AccountRepo struct{ db *pgxpool.Pool }

func NewAccountRepo(db *pgxpool.Pool) *AccountRepo { return &AccountRepo{db: db} }

func (r *AccountRepo) FindByID(ctx context.Context, id account.AccountID) (*account.Account, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, owner_name, currency, status, created_at, updated_at FROM accounts WHERE id = $1`,
		id.String(),
	)
	return scanAccount(row)
}

func (r *AccountRepo) Save(ctx context.Context, a *account.Account) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO accounts (id, owner_name, currency, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO UPDATE SET
			owner_name = EXCLUDED.owner_name,
			status     = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at`,
		a.ID.String(), a.OwnerName, string(a.Currency), string(a.Status), a.CreatedAt, a.UpdatedAt,
	)
	return err
}

// LockForUpdate acquires a SELECT FOR UPDATE row-level lock within the caller's
// transaction, preventing two concurrent transfers from passing the balance check.
func (r *AccountRepo) LockForUpdate(ctx context.Context, id account.AccountID) (*account.Account, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, owner_name, currency, status, created_at, updated_at FROM accounts WHERE id = $1 FOR UPDATE`,
		id.String(),
	)
	return scanAccount(row)
}

func scanAccount(row pgx.Row) (*account.Account, error) {
	var (
		id, ownerName, currency, status string
		createdAt, updatedAt            time.Time
	)
	err := row.Scan(&id, &ownerName, &currency, &status, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, account.ErrAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan account: %w", err)
	}
	accountID, err := account.ParseAccountID(id)
	if err != nil {
		return nil, err
	}
	return &account.Account{
		ID:        accountID,
		OwnerName: ownerName,
		Currency:  money.Currency(currency),
		Status:    account.Status(status),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
