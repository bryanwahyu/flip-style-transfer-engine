package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/transfer"
)

// EventStore implements port.TransferEventStore.
type EventStore struct{ db *pgxpool.Pool }

func NewEventStore(db *pgxpool.Pool) *EventStore { return &EventStore{db: db} }

func (s *EventStore) Append(ctx context.Context, evt transfer.TransferEvent) error {
	meta, err := json.Marshal(evt.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO transfer_events (id, transfer_id, event_type, state, amount, currency, occurred_at, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		evt.EventID, evt.TransferID.String(),
		string(evt.Type), string(evt.State),
		evt.Amount.Amount, string(evt.Amount.Currency),
		evt.OccurredAt, meta,
	)
	return err
}

func (s *EventStore) FindByTransferID(ctx context.Context, id transfer.TransferID) ([]transfer.TransferEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, transfer_id, event_type, state, amount, currency, occurred_at, metadata
		FROM transfer_events WHERE transfer_id = $1 ORDER BY occurred_at`, id.String())
	if err != nil {
		return nil, fmt.Errorf("query transfer events: %w", err)
	}
	defer rows.Close()

	var events []transfer.TransferEvent
	for rows.Next() {
		var (
			eventID, transferID, eventType, state string
			amount                                int64
			currency                              string
			occurredAt                            time.Time
			metaBytes                             []byte
		)
		if err := rows.Scan(&eventID, &transferID, &eventType, &state, &amount, &currency, &occurredAt, &metaBytes); err != nil {
			return nil, fmt.Errorf("scan transfer event: %w", err)
		}
		txID, err := transfer.ParseTransferID(transferID)
		if err != nil {
			return nil, err
		}
		var meta map[string]string
		json.Unmarshal(metaBytes, &meta) //nolint:errcheck

		events = append(events, transfer.TransferEvent{
			EventID: eventID, TransferID: txID,
			Type: transfer.EventType(eventType), State: transfer.State(state),
			Amount:     money.Money{Amount: amount, Currency: money.Currency(currency)},
			OccurredAt: occurredAt, Metadata: meta,
		})
	}
	return events, rows.Err()
}
