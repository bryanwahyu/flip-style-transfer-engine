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
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

// TransferRepo implements port.TransferRepository.
type TransferRepo struct{ db *pgxpool.Pool }

func NewTransferRepo(db *pgxpool.Pool) *TransferRepo { return &TransferRepo{db: db} }

func (r *TransferRepo) Save(ctx context.Context, t *transfer.Transfer) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO transfers (
			id, idempotency_key, source_account_id, dest_account_id,
			amount, currency, state, external_ref, failure_reason,
			debit_posted, credit_posted, created_at, updated_at, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		t.ID.String(), t.IdempotencyKey,
		t.SourceAccountID.String(), t.DestAccountID.String(),
		t.Amount.Amount, string(t.Amount.Currency),
		string(t.State), t.ExternalRef, t.FailureReason,
		t.DebitPosted, t.CreditPosted,
		t.CreatedAt, t.UpdatedAt, t.Version,
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
		       debit_posted, credit_posted, created_at, updated_at, version
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
		       debit_posted, credit_posted, created_at, updated_at, version
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
		SET state         = $1,
		    external_ref  = $2,
		    failure_reason= $3,
		    debit_posted  = $4,
		    credit_posted = $5,
		    updated_at    = $6,
		    version       = $7
		WHERE id = $8 AND version = $9`,
		string(t.State), t.ExternalRef, t.FailureReason,
		t.DebitPosted, t.CreditPosted,
		t.UpdatedAt, t.Version,
		t.ID.String(), t.Version-1, // optimistic lock: only update if version matches
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
		id, idempKey, srcID, dstID    string
		amount                         int64
		currency, state               string
		extRef, failReason            string
		debitPosted, creditPosted     bool
		createdAt, updatedAt          time.Time
		version                       int
	)
	err := row.Scan(
		&id, &idempKey, &srcID, &dstID,
		&amount, &currency, &state, &extRef, &failReason,
		&debitPosted, &creditPosted,
		&createdAt, &updatedAt, &version,
	)
	if err != nil {
		return nil, err
	}

	transferID, err := transfer.ParseTransferID(id)
	if err != nil {
		return nil, err
	}
	srcAccountID, err := account.ParseAccountID(srcID)
	if err != nil {
		return nil, err
	}
	dstAccountID, err := account.ParseAccountID(dstID)
	if err != nil {
		return nil, err
	}

	return &transfer.Transfer{
		ID:              transferID,
		IdempotencyKey:  idempKey,
		SourceAccountID: srcAccountID,
		DestAccountID:   dstAccountID,
		Amount:          money.Money{Amount: amount, Currency: money.Currency(currency)},
		State:           transfer.State(state),
		ExternalRef:     extRef,
		FailureReason:   failReason,
		DebitPosted:     debitPosted,
		CreditPosted:    creditPosted,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		Version:         version,
	}, nil
}
