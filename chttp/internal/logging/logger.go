package logging

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/iw2rmb/ploy/chttp/internal/config"
)

// Logger provides basic structured logging for CHTTP
type Logger struct {
	*slog.Logger
}

// NewLogger creates a simple structured logger
func NewLogger(cfg *config.Config) *Logger {
	// Set log level
	var level slog.Level
	switch cfg.Logging.Level {
	case "error":
		level = slog.LevelError
	case "warn":
		level = slog.LevelWarn
	case "info":
		level = slog.LevelInfo
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if cfg.Logging.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}

// LogCLIExecution logs CLI command execution
func (l *Logger) LogCLIExecution(ctx context.Context, command string, args []string, duration time.Duration, success bool, exitCode int, outputLength int) {
	l.Info("CLI command executed",
		"command", command,
		"args", args,
		"duration_ms", duration.Milliseconds(),
		"success", success,
		"exit_code", exitCode,
		"output_length", outputLength,
	)
}

// LogHTTPRequest logs HTTP request processing
func (l *Logger) LogHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration, clientIP string) {
	l.Info("HTTP request processed",
		"method", method,
		"path", path,
		"status_code", statusCode,
		"duration_ms", duration.Milliseconds(),
		"client_ip", clientIP,
	)
}

// LogError logs error events
func (l *Logger) LogError(ctx context.Context, msg string, err error, metadata map[string]interface{}) {
	attrs := []slog.Attr{
		slog.String("error", err.Error()),
	}

	for key, value := range metadata {
		attrs = append(attrs, slog.Any(key, value))
	}

	l.LogAttrs(ctx, slog.LevelError, msg, attrs...)
}

// LogAuthentication logs authentication events
func (l *Logger) LogAuthentication(ctx context.Context, success bool, clientIP string, reason string) {
	l.Info("Authentication attempt",
		"success", success,
		"client_ip", clientIP,
		"reason", reason,
	)
}