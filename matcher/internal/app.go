package internal

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/0xRichardL/vibe-copy-trading/matcher/internal/config"
	"github.com/0xRichardL/vibe-copy-trading/matcher/internal/kafka"
	"github.com/0xRichardL/vibe-copy-trading/matcher/internal/services"
	"github.com/0xRichardL/vibe-copy-trading/matcher/internal/store"
	redis "github.com/redis/go-redis/v9"
)

// App centralizes dependency wiring for the matcher service.
type App struct {
	cfg    config.Config
	logger *log.Logger

	redis         *redis.Client
	subscriptions *store.SubscriptionStore
	consumer      *kafka.SignalConsumer
	publisher     *kafka.ExecutionRequestPublisher
}

// NewApp builds an App with all required dependencies.
func NewApp(cfg config.Config, logger *log.Logger) *App {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	subStore := store.NewSubscriptionStore(redisClient, cfg.SubscriptionSetKey)
	consumer := kafka.NewSignalConsumer(cfg)
	publisher := kafka.NewExecutionRequestPublisher(cfg)

	return &App{
		cfg:           cfg,
		logger:        logger,
		redis:         redisClient,
		subscriptions: subStore,
		consumer:      consumer,
		publisher:     publisher,
	}
}

// Run starts the matcher service and blocks until ctx cancellation or fatal error.
func (a *App) Run(ctx context.Context) error {
	defer a.cleanup()

	matcher := services.NewMatcherService(a.subscriptions, a.consumer, a.publisher, a.logger)

	if err := matcher.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("matcher service exited with error: %w", err)
	}
	return ctx.Err()
}

func (a *App) cleanup() {
	if a.consumer != nil {
		if err := a.consumer.Close(); err != nil {
			a.logger.Printf("error closing Kafka consumer: %v", err)
		}
	}
	if a.publisher != nil {
		if err := a.publisher.Close(); err != nil {
			a.logger.Printf("error closing Kafka publisher: %v", err)
		}
	}
	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			a.logger.Printf("error closing Redis client: %v", err)
		}
	}
}
