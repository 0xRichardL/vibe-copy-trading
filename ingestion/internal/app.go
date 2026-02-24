package internal

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// App wires together configuration, Redis, Hyperliquid client, and Kafka
// publisher to implement the ingestion flow described in TECHNICAL_SPECS.md.
type App struct {
	cfg    Config
	logger *log.Logger

	store     *InfluencerStore
	client    *HyperliquidClient
	publisher *SignalPublisher
}

func NewApp(ctx context.Context, cfg Config, logger *log.Logger) (*App, error) {
	store := NewInfluencerStore(cfg)
	client := NewHyperliquidClient(cfg, logger)
	publisher := NewSignalPublisher(cfg)

	return &App{
		cfg:       cfg,
		logger:    logger,
		store:     store,
		client:    client,
		publisher: publisher,
	}, nil
}

// Run starts the ingestion loops for all configured influencers.
func (a *App) Run(ctx context.Context) error {
	defer a.cleanup()

	influencers, err := a.store.ListInfluencers(ctx)
	if err != nil {
		return err
	}

	if len(influencers) == 0 {
		a.logger.Println("no influencers configured; exiting")
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	for _, inf := range influencers {
		wg.Go(func() {
			if err := a.runForInfluencer(ctx, inf); err != nil {
				a.logger.Printf("ingestion loop for influencer %s exited: %v", inf.ID, err)
			}
		})
	}

	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

func (a *App) runForInfluencer(ctx context.Context, inf Influencer) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		handler := func(raw RawHyperliquidEvent, received time.Time) error {
			if raw == nil {
				return nil
			}

			sig, err := NormalizeEventToSignal(inf, raw, received)
			if err != nil {
				return err
			}

			ctxPub, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return a.publisher.Publish(ctxPub, sig)
		}

		if err := a.client.SubscribeAccountEvents(ctx, inf, handler); err != nil {
			a.logger.Printf("SubscribeAccountEvents error for %s: %v; retrying after backoff", inf.ID, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (a *App) cleanup() {
	if err := a.store.Close(); err != nil {
		a.logger.Printf("error closing Redis client: %v", err)
	}
	if err := a.publisher.Close(); err != nil {
		a.logger.Printf("error closing Kafka publisher: %v", err)
	}
}

// buildEventFingerprint remains available for potential downstream dedupe usage.
func buildEventFingerprint(inf Influencer, ev RawHyperliquidEvent) string {
	if ev == nil {
		return ""
	}

	parsed, err := parseHyperliquidUserEvent(ev, time.Time{})
	if err != nil {
		return ""
	}

	if parsed.SourceEventID != "" {
		return fmt.Sprintf("%s|%s|%s", inf.ID, parsed.Market, parsed.SourceEventID)
	}
	if parsed.Sequence != 0 {
		return fmt.Sprintf("%s|%s|%d", inf.ID, parsed.Market, parsed.Sequence)
	}
	return ""
}
