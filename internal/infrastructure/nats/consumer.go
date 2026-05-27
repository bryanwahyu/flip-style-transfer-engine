package nats

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// MessageHandler processes a single NATS message.
// It must be idempotent: the same message may be delivered more than once.
type MessageHandler func(ctx context.Context, subject string, data []byte) error

// Consumer subscribes to a JetStream stream with durable, at-least-once semantics.
type Consumer struct {
	js       jetstream.JetStream
	stream   string
	consumer string
	log      *slog.Logger
}

func NewConsumer(nc *nats.Conn, stream, consumer string, log *slog.Logger) (*Consumer, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}
	return &Consumer{js: js, stream: stream, consumer: consumer, log: log}, nil
}

// EnsureStream creates the stream if it does not exist.
func (c *Consumer) EnsureStream(ctx context.Context, subjects []string) error {
	_, err := c.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     c.stream,
		Subjects: subjects,
	})
	if err != nil {
		return fmt.Errorf("ensure stream %s: %w", c.stream, err)
	}
	return nil
}

// Subscribe starts consuming messages. Blocks until ctx is cancelled.
func (c *Consumer) Subscribe(ctx context.Context, handler MessageHandler) error {
	cons, err := c.js.CreateOrUpdateConsumer(ctx, c.stream, jetstream.ConsumerConfig{
		Name:    c.consumer,
		Durable: c.consumer,
	})
	if err != nil {
		return fmt.Errorf("create consumer %s: %w", c.consumer, err)
	}

	msgCtx, err := cons.Messages()
	if err != nil {
		return fmt.Errorf("get messages context: %w", err)
	}
	defer msgCtx.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msg, err := msgCtx.Next()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.log.Warn("consumer: error fetching message", "error", err)
			continue
		}

		if err := handler(ctx, msg.Subject(), msg.Data()); err != nil {
			c.log.Error("consumer: handler error, nacking", "subject", msg.Subject(), "error", err)
			msg.Nak() //nolint:errcheck
			continue
		}
		msg.Ack() //nolint:errcheck
	}
}
