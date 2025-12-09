package status

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/worker/lifecycle"
)

// SnapshotSource exposes cached lifecycle status snapshots.
// Returns typed NodeStatus for compile-time safety; callers convert
// to map[string]any via ToMap() only at serialization boundaries.
type SnapshotSource interface {
	LatestStatus() (lifecycle.NodeStatus, bool)
}

// Options configure the status provider.
type Options struct {
	Role   string
	Source SnapshotSource
}

// Provider surfaces node status information.
type Provider struct {
	role   string
	source SnapshotSource
}

// New constructs a status provider.
func New(opts Options) *Provider {
	role := strings.TrimSpace(opts.Role)
	if role == "" {
		role = "unified"
	}
	return &Provider{
		role:   role,
		source: opts.Source,
	}
}

// Snapshot returns the current node status.
// Uses typed NodeStatus internally and converts to map[string]any at the
// serialization boundary via ToMap(). This maintains wire-format compatibility
// while providing type safety throughout the status pipeline.
func (p *Provider) Snapshot(context.Context) (map[string]any, error) {
	if p.source != nil {
		// Use typed LatestStatus() accessor and convert to map at serialization boundary.
		if status, ok := p.source.LatestStatus(); ok {
			return status.ToMap(), nil
		}
	}
	// Fallback when no cached status is available (e.g., during startup).
	host, _ := os.Hostname()
	return map[string]any{
		"state":      "ok",
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"role":       p.role,
		"hostname":   host,
		"go_version": runtime.Version(),
	}, nil
}
