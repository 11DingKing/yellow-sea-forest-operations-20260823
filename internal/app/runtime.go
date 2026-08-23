package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/clock"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/config"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/httpapi"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/storage/sqlite"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/worker"
)

type Runtime struct {
	config   config.Config
	logger   *slog.Logger
	database *sqlite.Database
	service  *service.Service
	server   *http.Server
	worker   *worker.Runner
	close    sync.Once
}

func Build(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Runtime, error) {
	if logger == nil {
		logger = slog.Default()
	}
	database, err := sqlite.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open application database: %w", err)
	}
	realClock := clock.System{}
	svc := service.New(database, realClock, cfg.SessionTTL)
	if cfg.BootstrapEmail != "" {
		if _, err := svc.EnsureBootstrapAdmin(ctx, cfg.BootstrapEmail, cfg.BootstrapPass); err != nil {
			database.Close()
			return nil, fmt.Errorf("bootstrap administrator: %w", err)
		}
	}
	dispatcher := worker.NewDispatcher(nil, logger)
	runner, err := worker.New(database.Store(), dispatcher, realClock, logger, worker.Config{
		PollInterval: cfg.WorkerInterval, Lease: cfg.WorkerLease, JobTimeout: cfg.WorkerTimeout,
		BatchSize: cfg.WorkerBatchSize, Concurrency: cfg.WorkerCount,
	})
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("build outbox worker: %w", err)
	}
	api := httpapi.New(svc, database, logger)
	server := &http.Server{
		Addr: cfg.Address, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	return &Runtime{config: cfg, logger: logger, database: database, service: svc, server: server, worker: runner}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	background, cancel := context.WithCancel(ctx)
	defer cancel()
	errorsChannel := make(chan error, 3)
	workerDone := make(chan struct{})
	cleanupDone := make(chan struct{})
	go func() {
		r.logger.Info("http server starting", "address", r.server.Addr)
		if err := r.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsChannel <- fmt.Errorf("serve HTTP: %w", err)
		}
	}()
	go func() {
		defer close(workerDone)
		if err := r.worker.Run(background); err != nil && !errors.Is(err, context.Canceled) {
			errorsChannel <- fmt.Errorf("run outbox worker: %w", err)
		}
	}()
	go func() {
		defer close(cleanupDone)
		r.cleanupSessions(background)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		r.logger.Info("shutdown requested")
	case runErr = <-errorsChannel:
		r.logger.Error("application component stopped", "error", runErr)
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), r.config.ShutdownTimeout)
	defer shutdownCancel()
	if err := r.server.Shutdown(shutdownCtx); err != nil {
		r.server.Close()
		if runErr == nil {
			runErr = fmt.Errorf("shutdown HTTP server: %w", err)
		}
	}
	if err := waitForComponents(shutdownCtx, map[string]<-chan struct{}{"outbox worker": workerDone, "session cleanup": cleanupDone}); err != nil && runErr == nil {
		runErr = err
	}
	if err := r.Close(); err != nil && runErr == nil {
		runErr = err
	}
	return runErr
}

func waitForComponents(ctx context.Context, components map[string]<-chan struct{}) error {
	for name, done := range components {
		select {
		case <-done:
		case <-ctx.Done():
			return fmt.Errorf("wait for %s shutdown: %w", name, ctx.Err())
		}
	}
	return nil
}

func (r *Runtime) cleanupSessions(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			operationCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			removed, err := r.service.CleanupSessions(operationCtx, 500)
			cancel()
			if err != nil {
				r.logger.WarnContext(ctx, "session cleanup failed", "error", err)
				continue
			}
			if removed > 0 {
				r.logger.InfoContext(ctx, "expired sessions removed", "count", removed)
			}
		}
	}
}

func (r *Runtime) Close() error {
	var closeErr error
	r.close.Do(func() {
		if err := r.database.Close(); err != nil {
			closeErr = fmt.Errorf("close database: %w", err)
		}
	})
	return closeErr
}
