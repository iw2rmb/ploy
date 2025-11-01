package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/store"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "/etc/ploy/ployd.yaml", "Path to ployd configuration")
	flag.Parse()

	// Configure structured logger early (will be reconfigured after loading config).
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{})))

	// Load configuration from file.
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("load config", "err", err, "path", configPath)
		os.Exit(1)
	}

	// Reconfigure logger based on config.
	if err := initLogging(cfg.Logging); err != nil {
		slog.Error("initialize logging", "err", err)
		os.Exit(1)
	}

	// Resolve PostgreSQL DSN from environment or config.
	dsn := resolvePgDSN(cfg)
	if dsn == "" {
		slog.Error("postgresql dsn not configured", "hint", "set PLOY_SERVER_PG_DSN or configure postgres.dsn in config file")
		os.Exit(1)
	}

	// Initialize store.
	ctx := context.Background()
	st, err := store.NewStore(ctx, dsn)
	if err != nil {
		slog.Error("initialize store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	slog.Info("ployd server starting", "config", configPath)

	// Set up signal handling for graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run server.
	if err := run(ctx, cfg, st); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}

	slog.Info("ployd server stopped")
}

// run executes the main server loop and blocks until the context is canceled.
func run(ctx context.Context, cfg config.Config, st store.Store) error {
	// Wait for shutdown signal.
	<-ctx.Done()

	// Create a timeout context for graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Info("graceful shutdown initiated", "timeout", "10s")

	// TODO: Stop HTTP servers, background workers, etc.
	// This will be expanded in subsequent ROADMAP tasks.
	_ = shutdownCtx

	return nil
}

// resolvePgDSN returns the PostgreSQL DSN from environment or config.
// Precedence: PLOY_SERVER_PG_DSN > PLOY_POSTGRES_DSN > config.postgres.dsn
func resolvePgDSN(cfg config.Config) string {
	if dsn := strings.TrimSpace(os.Getenv("PLOY_SERVER_PG_DSN")); dsn != "" {
		return dsn
	}
	if dsn := strings.TrimSpace(os.Getenv("PLOY_POSTGRES_DSN")); dsn != "" {
		return dsn
	}
	return strings.TrimSpace(cfg.Postgres.DSN)
}

// initLogging configures the global slog logger based on the logging config.
func initLogging(cfg config.LoggingConfig) error {
	level := parseLogLevel(cfg.Level)
	opts := &slog.HandlerOptions{
		Level: level,
	}

	var w io.Writer = os.Stderr
	if cfg.File != "" {
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		// Note: file is not closed; it will be closed when the process exits.
		w = f
	}

	var handler slog.Handler
	if cfg.JSON {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	// Add static fields if configured.
	if len(cfg.StaticFields) > 0 {
		attrs := make([]slog.Attr, 0, len(cfg.StaticFields))
		for k, v := range cfg.StaticFields {
			attrs = append(attrs, slog.String(k, v))
		}
		handler = handler.WithAttrs(attrs)
	}

	slog.SetDefault(slog.New(handler))
	return nil
}

// parseLogLevel converts a string log level to slog.Level.
func parseLogLevel(levelStr string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
