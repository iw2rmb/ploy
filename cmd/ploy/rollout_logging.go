package main

import (
	"log/slog"
	"os"
)

// initRolloutLogger configures structured logging for rollout operations.
// Returns a logger with JSON output for machine-readable logs.
func initRolloutLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewJSONHandler(os.Stderr, opts)
	return slog.New(handler)
}

// logRolloutStep logs a structured event for a rollout step.
func logRolloutStep(logger *slog.Logger, step, status string, attrs ...any) {
	logger.Info("rollout_step",
		append([]any{"step", step, "status", status}, attrs...)...)
}

// logRolloutError logs a structured error event for a rollout step.
func logRolloutError(logger *slog.Logger, step string, err error, attrs ...any) {
	logger.Error("rollout_step",
		append([]any{"step", step, "status", "failed", "error", err.Error()}, attrs...)...)
}
