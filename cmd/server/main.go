package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/app"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/config"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/observability"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger := observability.NewLogger(cfg, os.Stdout)
	slog.SetDefault(logger)
	if err := os.MkdirAll("data", 0o750); err != nil {
		return fmt.Errorf("create local data directory: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := app.Build(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer runtime.Close()
	return runtime.Run(ctx)
}
