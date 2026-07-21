package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/honeybbq/tsdns/core"
	"github.com/honeybbq/tsdns/internal/api"
	"github.com/honeybbq/tsdns/internal/config"
	"github.com/honeybbq/tsdns/internal/metrics"
	"github.com/honeybbq/tsdns/internal/storage"
	"github.com/spf13/cobra"
)

// newServeCommand creates the 'serve' command to run the TSDNS server and admin API.
func newServeCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the TSDNS server and the admin API",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe(*configPath)
		},
	}

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	return cmd
}

func runServe(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	// Initialize the global logger
	setupLogger(cfg.Log)

	repo, _, err := storage.Open(cfg.Storage.DSN)
	if err != nil {
		return err
	}

	cacheInterval, err := time.ParseDuration(cfg.TSDNS.CacheRefreshInterval)
	if err != nil {
		return fmt.Errorf("invalid tsdns.cache_refresh_interval: %w", err)
	}

	s := tsdns.NewServerAddr(cfg.TSDNS.Listen).
		WithRepository(repo).
		WithCacheRefreshInterval(cacheInterval).
		WithLogger(slog.Default()).
		WithMetrics(metrics.NewPrometheusMetrics()).
		MustBuild()

	const errChSize = 2
	errCh := make(chan error, errChSize)

	// Start the TSDNS protocol server
	go func() {
		startErr := s.Start()
		if startErr != nil {
			errCh <- fmt.Errorf("TSDNS protocol server failed: %w", startErr)
		}
	}()

	// Start the admin API server (Mandatory)
	apiServer := startAdminAPI(cfg, s, errCh)

	// Wait for termination signal or server error
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		slog.Default().Info("Shutting down servers...")
	case serverErr := <-errCh:
		if serverErr != nil {
			// Panic with detailed information if a server fails to start (e.g. port in use)
			panic(fmt.Sprintf("CRITICAL SERVER ERROR: %v", serverErr))
		}
	}

	// Graceful shutdown with timeout
	const shutdownTimeout = 5 * time.Second
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if apiServer != nil {
		_ = apiServer.Shutdown(shutdownCtx)
	}

	return s.Close()
}

func startAdminAPI(cfg config.Config, s *tsdns.Server, errCh chan error) *api.Server {
	apiServer := api.New(cfg.API.Listen, cfg.API.Socket, cfg.API.Token, s)
	go func() {
		startErr := apiServer.ListenAndServe()
		if startErr != nil {
			errCh <- fmt.Errorf("admin API server failed: %w", startErr)
		}
	}()

	return apiServer
}

// setupLogger configures the global slog logger based on the provided configuration.
func setupLogger(cfg config.LogConfig) {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	if strings.EqualFold(cfg.Format, "json") {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
