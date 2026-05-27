package nats

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Publisher implements port.EventBus using NATS JetStream.
// It provides at-least-once delivery: messages are durable and survive restarts.
type Publisher struct {
	js jetstream.JetStream
}

func NewPublisher(nc *nats.Conn) (*Publisher, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}
	return &Publisher{js: js}, nil
}

func (p *Publisher) Publish(ctx context.Context, subject string, payload []byte) error {
	_, err := p.js.Publish(ctx, subject, payload)
	if err != nil {
		return fmt.Errorf("jetstream publish to %s: %w", subject, err)
	}
	return nil
}
