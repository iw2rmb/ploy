package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/api/config"
)

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
