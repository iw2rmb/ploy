package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// GetStatusCommand retrieves detailed status for a single run using
// the batch summary view (ID, repo refs, repo counts).
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("run status: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("run status: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return domaintypes.RunSummary{}, httpx.WrapError("run status", resp.Status, resp.Body)
	}

	var summary domaintypes.RunSummary
	if err := httpx.DecodeJSON(resp.Body, &summary, httpx.MaxJSONBodyBytes); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("run status: decode response: %w", err)
	}

	return summary, nil
}
