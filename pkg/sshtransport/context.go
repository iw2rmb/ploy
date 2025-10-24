package sshtransport

import (
	"context"
	"strings"
)

type contextKey string

const (
	jobContextKey  contextKey = "sshtransport-job"
	nodeContextKey contextKey = "sshtransport-node"
)

// WithJob annotates the context with the workflow job/run identifier.
func WithJob(ctx context.Context, jobID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	trimmed := strings.TrimSpace(jobID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, jobContextKey, trimmed)
}

// JobFromContext extracts the workflow job/run identifier previously attached via WithJob.
func JobFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if value, ok := ctx.Value(jobContextKey).(string); ok && strings.TrimSpace(value) != "" {
		return value, true
	}
	return "", false
}

// WithNode annotates the context with an explicit node identifier to target.
func WithNode(ctx context.Context, nodeID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	trimmed := strings.TrimSpace(nodeID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, nodeContextKey, trimmed)
}

// NodeFromContext retrieves the explicit node identifier, if present.
func NodeFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if value, ok := ctx.Value(nodeContextKey).(string); ok && strings.TrimSpace(value) != "" {
		return value, true
	}
	return "", false
}
