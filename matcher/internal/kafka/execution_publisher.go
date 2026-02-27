package kafka

import (
	"context"
	"fmt"

	busv1 "github.com/0xRichardL/vibe-copy-trading/libs/go/domain/bus/v1"
	"github.com/0xRichardL/vibe-copy-trading/matcher/internal/config"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

// ExecutionRequestPublisher publishes ExecutionRequest messages to Kafka.
type ExecutionRequestPublisher struct {
	writer *kafka.Writer
	Topic  string
}

// NewExecutionRequestPublisher creates a new Kafka publisher for execution requests.
func NewExecutionRequestPublisher(cfg config.Config) *ExecutionRequestPublisher {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(cfg.KafkaBrokers...),
		Topic:                  cfg.KafkaTopicExecRequests,
		RequiredAcks:           kafka.RequireAll,
		Balancer:               &kafka.Hash{},
		AllowAutoTopicCreation: true,
	}
	return &ExecutionRequestPublisher{writer: writer, Topic: cfg.KafkaTopicExecRequests}
}

// Publish sends an ExecutionRequest to the configured Kafka topic.
func (p *ExecutionRequestPublisher) Publish(ctx context.Context, r *busv1.ExecutionRequest) error {
	value, err := proto.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal execution request proto: %w", err)
	}

	key := []byte(r.GetSubscriberId())
	if len(key) == 0 {
		key = []byte(r.GetInfluencerId())
	}

	msg := kafka.Message{
		Key:   key,
		Value: value,
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("kafka write: %w", err)
	}
	return nil
}

// Close closes the underlying Kafka writer.
func (p *ExecutionRequestPublisher) Close() error {
	return p.writer.Close()
}
