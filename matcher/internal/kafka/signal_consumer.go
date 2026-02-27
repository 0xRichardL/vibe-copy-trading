package kafka

import (
	"context"
	"errors"
	"fmt"

	busv1 "github.com/0xRichardL/vibe-copy-trading/libs/go/domain/bus/v1"
	"github.com/0xRichardL/vibe-copy-trading/matcher/internal/config"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

// SignalConsumer consumes normalized influencer signals from Kafka.
type SignalConsumer struct {
	reader *kafka.Reader
}

// NewSignalConsumer creates a new Kafka consumer for influencer signals.
func NewSignalConsumer(cfg config.Config) *SignalConsumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: cfg.KafkaBrokers,
		GroupID: cfg.KafkaGroupID,
		Topic:   cfg.KafkaTopicSignals,
	})
	return &SignalConsumer{reader: reader}
}

// Consume reads messages from Kafka and passes them to the provided handler.
func (c *SignalConsumer) Consume(ctx context.Context, handler func(context.Context, *busv1.Signal) error) error {
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ctx.Err()
			}
			return fmt.Errorf("kafka read: %w", err)
		}

		var sig busv1.Signal
		if err := proto.Unmarshal(msg.Value, &sig); err != nil {
			return fmt.Errorf("unmarshal signal proto: %w", err)
		}

		if err := handler(ctx, &sig); err != nil {
			return err
		}
	}
}

// Close closes the underlying Kafka reader.
func (c *SignalConsumer) Close() error {
	return c.reader.Close()
}
