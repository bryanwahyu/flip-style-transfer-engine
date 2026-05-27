package transfer

import (
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/domain/money"
)

// EventType enumerates the domain events emitted during a transfer's lifecycle.
type EventType string

const (
	EventTransferRequested  EventType = "transfer.requested"
	EventTransferDebited    EventType = "transfer.debited"
	EventTransferBankCalled EventType = "transfer.bank_called"
	EventTransferCredited   EventType = "transfer.credited"
	EventTransferCompleted  EventType = "transfer.completed"
	EventTransferFailed     EventType = "transfer.failed"
)

// TransferEvent is the envelope stored in transfer_events and published to NATS.
type TransferEvent struct {
	EventID    string
	TransferID TransferID
	Type       EventType
	State      State
	Amount     money.Money
	OccurredAt time.Time
	Metadata   map[string]string
}

func NewEvent(t *Transfer, typ EventType) TransferEvent {
	return TransferEvent{
		EventID:    NewTransferID().String(), // reuse ID generation for simplicity
		TransferID: t.ID,
		Type:       typ,
		State:      t.State,
		Amount:     t.Amount,
		OccurredAt: time.Now().UTC(),
		Metadata:   map[string]string{},
	}
}
