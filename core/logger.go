package tsdns

import (
	"log/slog"
	"os"
)

// NewDefaultLogger returns a new standard output logger based on slog.
func NewDefaultLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}
