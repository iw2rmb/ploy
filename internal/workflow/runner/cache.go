package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// CacheComposeRequest describes the inputs required to compose a cache key.
type CacheComposeRequest struct {
	Stage  Stage
	Ticket contracts.WorkflowTicket
}

// CacheComposer builds deterministic cache keys for workflow stages.
type CacheComposer interface {
	Compose(ctx context.Context, req CacheComposeRequest) (string, error)
}

type defaultCacheComposer struct{}

// Compose builds the default cache key using stage lane, manifest version, and toggles.
func (defaultCacheComposer) Compose(ctx context.Context, req CacheComposeRequest) (string, error) {
	_ = ctx
	lane := strings.TrimSpace(req.Stage.Lane)
	if lane == "" {
		return "", fmt.Errorf("lane missing")
	}
	manifest := strings.TrimSpace(req.Stage.Constraints.Manifest.Manifest.Version)
	if manifest == "" {
		manifest = "unknown"
	}
	toggles := "none"
	if len(req.Stage.Aster.Toggles) > 0 {
		toggles = strings.Join(req.Stage.Aster.Toggles, "+")
	}
	return fmt.Sprintf("%s/%s@manifest=%s@aster=%s", lane, lane, manifest, toggles), nil
}
