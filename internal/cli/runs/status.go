package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// GetStatusCommand retrieves detailed status for a single run using
// the batch summary view (ID, repo refs, repo counts).
type GetStatusCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
}

// Run delegates to the Mods batch client to fetch run status from
// GET /v1/runs/{id} and returns the BatchSummary.
func (c GetStatusCommand) Run(ctx context.Context) (mods.BatchSummary, error) {
	if c.Client == nil {
		return mods.BatchSummary{}, fmt.Errorf("run status: http client required")
	}
	if c.BaseURL == nil {
		return mods.BatchSummary{}, fmt.Errorf("run status: base url required")
	}
	if c.RunID.IsZero() {
		return mods.BatchSummary{}, fmt.Errorf("run status: run id required")
	}

	cmd := mods.GetRunStatusCommand{
		Client:  c.Client,
		BaseURL: c.BaseURL,
		RunID:   c.RunID,
	}
	return cmd.Run(ctx)
}
