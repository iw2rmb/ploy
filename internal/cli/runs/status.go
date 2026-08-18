package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// GetStatusCommand retrieves detailed status for a single run using
// the run summary view (ID, repo refs, repo counts).
type GetStatusCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
}

// Run executes GET /v1/runs/{id} and returns the run domaintypes.RunSummary.
func (c GetStatusCommand) Run(ctx context.Context) (domaintypes.RunSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("run status: %w", err)
	}
	if c.RunID.IsZero() {
		return domaintypes.RunSummary{}, fmt.Errorf("run status: run id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String())
	return httpx.DoJSON[domaintypes.RunSummary](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "run status")
}
