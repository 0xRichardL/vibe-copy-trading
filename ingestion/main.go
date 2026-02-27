package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	service "github.com/0xRichardL/vibe-copy-trading/ingestion/internal"
	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/config"
)

func main() {
	logger := log.New(os.Stdout, "ingestion ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	app := service.NewApp(cfg, logger)

	if err := app.Run(ctx); err != nil {
		logger.Fatalf("service exited with error: %v", err)
	}
}
