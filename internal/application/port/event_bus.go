package port

import "context"

// EventBus publishes domain events to the message broker (NATS JetStream).
// Implementations must be safe for concurrent calls.
type EventBus interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}
