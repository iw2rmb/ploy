package step

import (
	"context"
	"strings"
)

type gateShareDirKey struct{}

// WithGateShareDir attaches an optional host share directory to gate execution
// context. When set, dockerGateExecutor mounts it at /share.
func WithGateShareDir(ctx context.Context, shareDir string) context.Context {
	shareDir = strings.TrimSpace(shareDir)
	if shareDir == "" {
		return ctx
	}
	return context.WithValue(ctx, gateShareDirKey{}, shareDir)
}

func gateShareDirFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	shareDir, _ := ctx.Value(gateShareDirKey{}).(string)
	return strings.TrimSpace(shareDir)
}
