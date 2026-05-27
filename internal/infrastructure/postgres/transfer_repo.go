package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/ledger"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

// TransferRepo implements port.TransferRepository against PostgreSQL.
type TransferRepo struct {
	db *pgxpool.Pool
}

func NewTransferRepo(db *pgxpool.Pool) *TransferRepo {
	return &TransferRepo{db: db}
}

func (r *TransferRepo) Save(ctx context.Context, t *transfer.Transfer) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO transfers (
			id, idempotency_key, source_account_id, dest_account_id,
			amount, currency, state, external_ref, failure_reason,
			debit_tx_id, credit_tx_id, created_at, updated_at, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		t.ID.String(),
		t.IdempotencyKey,
		t.SourceAccountID.String(),
		t.DestAccountID.String(),
		t.Amount.Amount,
		string(t.Amount.Currency),
		string(t.State),
		t.ExternalRef,
		t.FailureReason,
		nullableUUID(t.DebitTxID.UUID.String()),
		nullableUUID(t.CreditTxID.UUID.String()),
		t.CreatedAt,
		t.UpdatedAt,
		t.Version,
	)
	if err != nil {
		return fmt.Errorf("insert transfer: %w", err)
	}
	return nil
}

func (r *TransferRepo) FindByID(ctx context.Context, id transfer.TransferID) (*transfer.Transfer, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, idempotency_key, source_account_id, dest_account_id,
		       amount, currency, state, external_ref, failure_reason,
		       debit_tx_id, credit_tx_id, created_at, updated_at, version
		FROM transfers WHERE id = $1`, id.String())

	t, err := scanTransfer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, transfer.ErrTransferNotFound
	}
	return t, err
}

func (r *TransferRepo) FindByIdempotencyKey(ctx context.Context, key string) (*transfer.Transfer, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, idempotency_key, source_account_id, dest_account_id,
		       amount, currency, state, external_ref, failure_reason,
		       debit_tx_id, credit_tx_id, created_at, updated_at, version
		FROM transfers WHERE idempotency_key = $1`, key)

	t, err := scanTransfer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (r *TransferRepo) UpdateState(ctx context.Context, t *transfer.Transfer) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE transfers
		SET state = $1, external_ref = $2, failure_reason = $3,
		    debit_tx_id = $4, credit_tx_id = $5, updated_at = $6, version = $7
		WHERE id = $8 AND version = $9`,
		string(t.State), t.ExternalRef, t.FailureReason,
		nullableUUID(t.DebitTxID.UUID.String()),
		nullableUUID(t.CreditTxID.UUID.String()),
		t.UpdatedAt, t.Version,
		t.ID.String(), t.Version-1, // optimistic lock: only update if version matches previous
	)
	if err != nil {
		return fmt.Errorf("update transfer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("optimistic lock conflict on transfer %s", t.ID.String())
	}
	return nil
}

func scanTransfer(row pgx.Row) (*transfer.Transfer, error) {
	var (
		id, idempKey, srcID, dstID string
		amount                     int64
		currency                   string
		state                      string
		extRef, failReason         string
		debitTxID, creditTxID      *string
		createdAt, updatedAt       time.Time
		version                    int
	)
	err := row.Scan(
		&id, &idempKey, &srcID, &dstID,
		&amount, &currency, &state, &extRef, &failReason,
		&debitTxID, &creditTxID, &createdAt, &updatedAt, &version,
	)
	if err != nil {
		return nil, err
	}

	transferID, err := transfer.ParseTransferID(id)
	if err != nil {
		return nil, err
	}
	srcAccountID, err := ledger.ParseAccountID(srcID)
	if err != nil {
		return nil, err
	}
	dstAccountID, err := ledger.ParseAccountID(dstID)
	if err != nil {
		return nil, err
	}
	money := ledger.Money{Amount: amount, Currency: ledger.Currency(currency)}

	t := &transfer.Transfer{
		ID:              transferID,
		IdempotencyKey:  idempKey,
		SourceAccountID: srcAccountID,
		DestAccountID:   dstAccountID,
		Amount:          money,
		State:           transfer.State(state),
		ExternalRef:     extRef,
		FailureReason:   failReason,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		Version:         version,
	}

	if debitTxID != nil {
		t.DebitTxID, _ = ledger.ParseTransactionID(*debitTxID)
	}
	if creditTxID != nil {
		t.CreditTxID, _ = ledger.ParseTransactionID(*creditTxID)
	}

	return t, nil
}

// nullableUUID returns nil for the zero UUID string "00000000-0000-0000-0000-000000000000".
func nullableUUID(s string) *string {
	zero := "00000000-0000-0000-0000-000000000000"
	if s == zero || s == "" {
		return nil
	}
	return &s
}
