package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address         string
	DatabaseURL     string
	SessionTTL      time.Duration
	WorkerInterval  time.Duration
	WorkerLease     time.Duration
	WorkerTimeout   time.Duration
	WorkerBatchSize int
	WorkerCount     int
	ShutdownTimeout time.Duration
	BootstrapEmail  string
	BootstrapPass   string
	LogLevel        slog.Level
}

func Load() (Config, error) {
	config := Config{
		Address:         env("FOREST_ADDR", ":8080"),
		DatabaseURL:     env("FOREST_DATABASE_URL", "file:data/forest-operations.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"),
		SessionTTL:      12 * time.Hour,
		WorkerInterval:  2 * time.Second,
		WorkerLease:     30 * time.Second,
		WorkerTimeout:   20 * time.Second,
		WorkerBatchSize: 20,
		WorkerCount:     4,
		ShutdownTimeout: 10 * time.Second,
		BootstrapEmail:  env("FOREST_BOOTSTRAP_ADMIN_EMAIL", ""),
		BootstrapPass:   env("FOREST_BOOTSTRAP_ADMIN_PASSWORD", ""),
		LogLevel:        slog.LevelInfo,
	}
	var err error
	if config.SessionTTL, err = duration("FOREST_SESSION_TTL", config.SessionTTL); err != nil {
		return Config{}, err
	}
	if config.WorkerInterval, err = duration("FOREST_WORKER_INTERVAL", config.WorkerInterval); err != nil {
		return Config{}, err
	}
	if config.WorkerLease, err = duration("FOREST_WORKER_LEASE", config.WorkerLease); err != nil {
		return Config{}, err
	}
	if config.WorkerTimeout, err = duration("FOREST_WORKER_TIMEOUT", config.WorkerTimeout); err != nil {
		return Config{}, err
	}
	if config.WorkerTimeout >= config.WorkerLease {
		return Config{}, fmt.Errorf("FOREST_WORKER_TIMEOUT must be shorter than FOREST_WORKER_LEASE")
	}
	if config.WorkerBatchSize, err = Int("FOREST_WORKER_BATCH_SIZE", config.WorkerBatchSize, 1, 100); err != nil {
		return Config{}, err
	}
	if config.WorkerCount, err = Int("FOREST_WORKER_COUNT", config.WorkerCount, 1, 32); err != nil {
		return Config{}, err
	}
	if config.ShutdownTimeout, err = duration("FOREST_SHUTDOWN_TIMEOUT", config.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	switch strings.ToLower(env("FOREST_LOG_LEVEL", "info")) {
	case "debug":
		config.LogLevel = slog.LevelDebug
	case "info":
		config.LogLevel = slog.LevelInfo
	case "warn":
		config.LogLevel = slog.LevelWarn
	case "error":
		config.LogLevel = slog.LevelError
	default:
		return Config{}, fmt.Errorf("FOREST_LOG_LEVEL is invalid")
	}
	if strings.TrimSpace(config.Address) == "" {
		return Config{}, fmt.Errorf("FOREST_ADDR cannot be empty")
	}
	if strings.TrimSpace(config.DatabaseURL) == "" {
		return Config{}, fmt.Errorf("FOREST_DATABASE_URL cannot be empty")
	}
	if (config.BootstrapEmail == "") != (config.BootstrapPass == "") {
		return Config{}, fmt.Errorf("bootstrap admin email and password must be configured together")
	}
	return config, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}

func Int(key string, fallback, minimum, maximum int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", key, minimum, maximum)
	}
	return parsed, nil
}
