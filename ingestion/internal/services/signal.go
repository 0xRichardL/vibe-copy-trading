package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/kafka"
	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/store"
	busv1 "github.com/0xRichardL/vibe-copy-trading/libs/go/domain/bus/v1"
	"github.com/0xRichardL/vibe-copy-trading/libs/go/routine"
)

const (
	defaultPollInterval = time.Second
)

// SignalService owns the background routines that fan out influencer streams.
type SignalService struct {
	store       *store.InfluencerStore
	hyperliquid *HyperliquidService
	publisher   *kafka.SignalPublisher

	once         sync.Once
	manager      *routine.Manager
	pollInterval time.Duration
}

func NewSignalService(store *store.InfluencerStore, hyperliquid *HyperliquidService, publisher *kafka.SignalPublisher) *SignalService {
	return &SignalService{
		store:        store,
		hyperliquid:  hyperliquid,
		publisher:    publisher,
		pollInterval: defaultPollInterval,
	}
}

// Start prepares the signal service for work; orchestration logic will be
// added in future revisions.
func (s *SignalService) Start(ctx context.Context) error {
	s.once.Do(func() {
		s.manager = routine.NewManager(ctx)
	})
	for {
		select {
		case <-ctx.Done():
			return s.manager.ShutdownAll()
		default:
		}

		inf, putBack, err := s.store.Acquire(ctx)
		if err != nil {
			if err == store.ErrNoInfluencers {
				select {
				case <-ctx.Done():
					return s.manager.ShutdownAll()
				case <-time.After(s.pollInterval):
				}
				continue
			}
			return fmt.Errorf("acquire influencer: %w", err)
		}

		err = s.manager.RunTask(&routine.Task{
			ID: inf.Address,
			Handler: func(taskCtx context.Context) error {
				if err := s.hyperliquid.SubscribeAccountEvents(taskCtx, inf, s.handleSignal); err != nil {
					return fmt.Errorf("subscribe to hyperliquid events: %w", err)
				}
				return nil
			},
			OnDone: func(id string) {
				if err := putBack(); err != nil {
					fmt.Printf("put back influencer %s: %v\n", inf.Address, err)
				}
			},
		})
		if err != nil {
			if err := putBack(); err != nil {
				fmt.Printf("put back influencer %s: %v\n", inf.Address, err)
			}
			return fmt.Errorf("run task: %w", err)
		}
	}
}

func (s *SignalService) handleSignal(ctx context.Context, sig *busv1.Signal) error {
	if sig == nil {
		return nil
	}

	ctxPub, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.publisher.Publish(ctxPub, sig)
}
