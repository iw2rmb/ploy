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

// Run executes GET /v1/runs/{id} and returns the run Summary.
func (c GetStatusCommand) Run(ctx context.Context) (Summary, error) {
	if c.Client == nil {
		return Summary{}, fmt.Errorf("run status: http client required")
	}
	if c.BaseURL == nil {
		return Summary{}, fmt.Errorf("run status: base url required")
	}
	if c.RunID.IsZero() {
		return Summary{}, fmt.Errorf("run status: run id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Summary{}, fmt.Errorf("run status: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return Summary{}, fmt.Errorf("run status: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Summary{}, httpx.WrapError("run status", resp.Status, resp.Body)
	}

	var summary Summary
	if err := httpx.DecodeJSON(resp.Body, &summary, httpx.MaxJSONBodyBytes); err != nil {
		return Summary{}, fmt.Errorf("run status: decode response: %w", err)
	}

	return summary, nil
}
