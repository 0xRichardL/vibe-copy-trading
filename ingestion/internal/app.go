package internal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	busv1 "github.com/0xRichardL/vibe-copy-trading/libs/go/domain/bus/v1"
	"github.com/0xRichardL/vibe-copy-trading/libs/go/routine"
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

// SignalHandler processes normalized signals prior to downstream distribution.
type SignalHandler func(context.Context, *busv1.Signal) error

func NewApp(cfg Config, logger *log.Logger) *App {
	store := NewInfluencerStore(cfg)
	client := NewHyperliquidClient(cfg, logger)
	publisher := NewSignalPublisher(cfg)

	return &App{
		cfg:       cfg,
		logger:    logger,
		store:     store,
		client:    client,
		publisher: publisher,
	}
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

	manager := routine.NewManager(ctx)

	for _, influencer := range influencers {
		inf := influencer
		task := &routine.Task{
			ID: inf.ID,
			Handler: func(taskCtx context.Context) error {
				return a.streamInfluencerEvents(taskCtx, inf)
			},
			OnError: func(id string, err error) {
				a.logger.Printf("ingestion loop for influencer %s exited: %v", id, err)
			},
		}
		if err := manager.RunTask(task); err != nil {
			return fmt.Errorf("start task for %s: %w", inf.ID, err)
		}
	}

	<-ctx.Done()

	for _, inf := range influencers {
		if err := manager.Shutdown(inf.ID); err != nil && !errors.Is(err, routine.ErrRoutineNotFound) {
			a.logger.Printf("graceful shutdown warning for %s: %v", inf.ID, err)
		}
	}

	return ctx.Err()
}

func (a *App) streamInfluencerEvents(ctx context.Context, inf Influencer) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := a.client.SubscribeAccountEvents(ctx, inf, a.handleSignal); err != nil {
			a.logger.Printf("SubscribeAccountEvents error for %s: %v; retrying after backoff", inf.ID, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (a *App) handleSignal(ctx context.Context, sig *busv1.Signal) error {
	if sig == nil {
		return nil
	}

	ctxPub, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return a.publisher.Publish(ctxPub, sig)
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
// It currently fingerprints by the normalized SignalId, which is already a
// stable hash of influencer, market, and source event identifier.
func buildEventFingerprint(sig *busv1.Signal) string {
	if sig == nil {
		return ""
	}
	if sig.SignalId != "" {
		return sig.SignalId
	}
	if sig.SourceEventId != "" {
		return fmt.Sprintf("%s|%s|%s", sig.InfluencerId, sig.Market, sig.SourceEventId)
	}
	return ""
}
