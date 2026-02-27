package internal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/config"
	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/kafka"
	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/rest"
	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/services"
	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/store"
	redis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

// App centralizes dependency wiring for the ingestion service.
type App struct {
	cfg    config.Config
	logger *log.Logger

	redis     *redis.Client
	store     *store.InfluencerStore
	publisher *kafka.SignalPublisher
	signal    *services.SignalService

	httpServer *http.Server
}

// NewApp builds an App with all required dependencies.
func NewApp(cfg config.Config, logger *log.Logger) *App {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	infStore := store.NewInfluencerStore(redisClient, cfg.InfluencerSetKey)
	publisher := kafka.NewSignalPublisher(cfg)
	client := services.NewHyperliquidService(cfg, logger)
	signal := services.NewStreamService(infStore, client, publisher)

	return &App{
		cfg:       cfg,
		logger:    logger,
		redis:     redisClient,
		store:     infStore,
		publisher: publisher,
		signal:    signal,
	}
}

// Run starts background services and blocks until ctx cancellation or fatal error.
func (a *App) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer a.cleanup()

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := a.signal.Start(gctx); err != nil {
			return fmt.Errorf("start stream service: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		return a.runHTTPServer(gctx)
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return ctx.Err()
}

func (a *App) runHTTPServer(ctx context.Context) error {
	r, srv := rest.NewServer(a.cfg)
	a.httpServer = srv
	infController := rest.NewInfluencerController(a.store)
	infController.RegisterInfluencerRoutes(r.Group(""))

	serverErr := make(chan error, 1)
	go func() {
		fmt.Printf("HTTP server started at: %s\n", srv.Addr)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	// App context shutdown:
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server shutdown: %w", err)
		}
		err := <-serverErr
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return ctx.Err()
	// HTTP server error:
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func (a *App) cleanup() {
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
