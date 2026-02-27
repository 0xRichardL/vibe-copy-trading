package services

import (
	"context"
	"fmt"
	"log"

	busv1 "github.com/0xRichardL/vibe-copy-trading/libs/go/domain/bus/v1"
	"github.com/0xRichardL/vibe-copy-trading/matcher/internal/kafka"
	"github.com/0xRichardL/vibe-copy-trading/matcher/internal/store"
)

// MatcherService owns the background routines that consume influencer signals
// and (in future iterations) fan them out into execution requests for subscribers.
type MatcherService struct {
	store     *store.SubscriptionStore
	consumer  *kafka.SignalConsumer
	publisher *kafka.ExecutionRequestPublisher
	logger    *log.Logger
}

// NewMatcherService constructs a MatcherService with its dependencies.
func NewMatcherService(store *store.SubscriptionStore, consumer *kafka.SignalConsumer, publisher *kafka.ExecutionRequestPublisher, logger *log.Logger) *MatcherService {
	return &MatcherService{
		store:     store,
		consumer:  consumer,
		publisher: publisher,
		logger:    logger,
	}
}

// Start begins consuming influencer signals. Matching and fan-out logic
// will be added in subsequent revisions to align with the TECHNICAL_SPECS.
func (s *MatcherService) Start(ctx context.Context) error {
	handler := func(ctx context.Context, sig *busv1.Signal) error {
		if sig == nil {
			return nil
		}

		s.logger.Printf("received signal for influencer %s", sig.GetInfluencerId())
		// TODO: resolve active subscriptions from s.store, apply filters,
		// build ExecutionRequest messages, and publish via s.publisher.
		return nil
	}

	if err := s.consumer.Consume(ctx, handler); err != nil {
		return fmt.Errorf("consume signals: %w", err)
	}
	return nil
}
