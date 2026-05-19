package main

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/daemonlog"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// initLogging configures the daemon JSON logger based on the logging config.
func initLogging(cfg config.LoggingConfig) error {
	daemonlog.ConfigureDefault(stdoutWriter, stderrWriter, parseLogLevel(cfg.Level), daemonlog.FromEnv())
	return nil
}

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
)

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
