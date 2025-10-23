package status

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/iw2rmb/ploy/internal/ployd/config"
)

// Options configure the status provider.
type Options struct {
	Mode string
}

// Provider surfaces node status information.
type Provider struct {
	mode string
}

// New constructs a status provider.
func New(opts Options) *Provider {
	mode := opts.Mode
	if mode == "" {
		mode = config.ModeWorker
	}
	return &Provider{mode: mode}
}

// Snapshot returns the current node status.
func (p *Provider) Snapshot(context.Context) (map[string]any, error) {
	host, _ := os.Hostname()
	return map[string]any{
		"state":      "ok",
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"mode":       p.mode,
		"hostname":   host,
		"go_version": runtime.Version(),
	}, nil
}
