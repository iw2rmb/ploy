package runs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

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
	if c.Client == nil {
		return StartResult{}, fmt.Errorf("run start: http client required")
	}
	if c.BaseURL == nil {
		return StartResult{}, fmt.Errorf("run start: base url required")
	}
	if c.RunID.IsZero() {
		return StartResult{}, fmt.Errorf("run start: run id required")
	}

	endpoint := c.BaseURL.JoinPath("/v1/runs", c.RunID.String(), "start")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return StartResult{}, fmt.Errorf("run start: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return StartResult{}, fmt.Errorf("run start: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return StartResult{}, fmt.Errorf("run start: %s", msg)
	}

	var result StartResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StartResult{}, fmt.Errorf("run start: decode response: %w", err)
	}

	return result, nil
}
