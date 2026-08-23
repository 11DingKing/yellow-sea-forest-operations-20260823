package observability

import (
	"io"
	"log/slog"
	"os"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/config"
)

func NewLogger(cfg config.Config, output io.Writer) *slog.Logger {
	if output == nil {
		output = os.Stdout
	}
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{Level: cfg.LogLevel, AddSource: cfg.LogLevel == slog.LevelDebug})
	return slog.New(handler).With("service", "shenzhen-forest-operations")
}
