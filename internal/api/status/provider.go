package status

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"
)

// SnapshotSource exposes cached lifecycle status snapshots.
type SnapshotSource interface {
	LatestStatus() (map[string]any, bool)
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
func (p *Provider) Snapshot(context.Context) (map[string]any, error) {
	if p.source != nil {
		if snapshot, ok := p.source.LatestStatus(); ok && len(snapshot) > 0 {
			return snapshot, nil
		}
	}
	host, _ := os.Hostname()
	return map[string]any{
		"state":      "ok",
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"role":       p.role,
		"hostname":   host,
		"go_version": runtime.Version(),
	}, nil
}
