package status

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"
)

// Options configure the status provider.
type Options struct {
	Role string
}

// Provider surfaces node status information.
type Provider struct {
	role string
}

// New constructs a status provider.
func New(opts Options) *Provider {
	role := strings.TrimSpace(opts.Role)
	if role == "" {
		role = "unified"
	}
	return &Provider{role: role}
}

// Snapshot returns the current node status.
func (p *Provider) Snapshot(context.Context) (map[string]any, error) {
	host, _ := os.Hostname()
	return map[string]any{
		"state":      "ok",
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"role":       p.role,
		"hostname":   host,
		"go_version": runtime.Version(),
	}, nil
}
