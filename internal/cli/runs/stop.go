package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// StopCommand stops a run and returns its domaintypes.RunSummary.
type StopCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
}

// Run executes POST /v1/runs/{id}/cancel to stop (cancel) the run.
func (c StopCommand) Run(ctx context.Context) (domaintypes.RunSummary, error) {
	var zero domaintypes.RunSummary

	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return zero, fmt.Errorf("run stop: %w", err)
	}
	if c.RunID.IsZero() {
		return zero, fmt.Errorf("run stop: run id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "cancel")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return zero, fmt.Errorf("run stop: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return zero, fmt.Errorf("run stop: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return zero, httpx.WrapError("run stop", resp.Status, resp.Body)
	}

	var summary domaintypes.RunSummary
	if err := httpx.DecodeJSON(resp.Body, &summary, httpx.MaxJSONBodyBytes); err != nil {
		return zero, fmt.Errorf("run stop: decode response: %w", err)
	}

	return summary, nil
}
