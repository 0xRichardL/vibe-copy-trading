package kafka

import (
	"context"
	"fmt"

	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/config"
	busv1 "github.com/0xRichardL/vibe-copy-trading/libs/go/domain/bus/v1"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

// SignalPublisher publishes normalized Signals to Kafka.
type SignalPublisher struct {
	writer *kafka.Writer
	Topic  string
}

func NewSignalPublisher(cfg config.Config) *SignalPublisher {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(cfg.KafkaBrokers...),
		Topic:                  cfg.KafkaTopic,
		RequiredAcks:           kafka.RequireAll,
		Balancer:               &kafka.Hash{},
		AllowAutoTopicCreation: true,
	}
	return &SignalPublisher{writer: writer, Topic: cfg.KafkaTopic}
}

func (p *SignalPublisher) Publish(ctx context.Context, s *busv1.Signal) error {
	value, err := proto.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal signal proto: %w", err)
	}

	msg := kafka.Message{
		Key:   []byte(s.GetInfluencerId()),
		Value: value,
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("kafka write: %w", err)
	}
	return nil
}

func (p *SignalPublisher) Close() error {
	return p.writer.Close()
}
