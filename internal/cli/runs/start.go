package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// StartCommand starts execution for pending repos in a run.
// It calls POST /v1/runs/{id}/start on the control plane.
type StartCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
}

// StartResult contains the result of starting a run.
type StartResult struct {
	RunID       domaintypes.RunID `json:"run_id"`
	Started     int               `json:"started"`
	AlreadyDone int               `json:"already_done"`
	Pending     int               `json:"pending"`
}

// Run executes POST /v1/runs/{id}/start to start execution for pending repos.
func (c StartCommand) Run(ctx context.Context) (StartResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return StartResult{}, fmt.Errorf("run start: %w", err)
	}
	if c.RunID.IsZero() {
		return StartResult{}, fmt.Errorf("run start: run id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "start")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return StartResult{}, fmt.Errorf("run start: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return StartResult{}, fmt.Errorf("run start: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return StartResult{}, httpx.WrapError("run start", resp.Status, resp.Body)
	}

	var result StartResult
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return StartResult{}, fmt.Errorf("run start: decode response: %w", err)
	}

	return result, nil
}
