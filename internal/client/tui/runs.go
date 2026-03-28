package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// ListRunsResult is the response from GET /v1/runs.
type ListRunsResult struct {
	Runs []domaintypes.RunSummary `json:"runs"`
}

// ListRunsCommand fetches a paginated list of runs.
type ListRunsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Limit   int32
	Offset  int32
}

// Run executes GET /v1/runs.
func (c ListRunsCommand) Run(ctx context.Context) (ListRunsResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return ListRunsResult{}, fmt.Errorf("list runs: %w", err)
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs")
	q := endpoint.Query()
	if c.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", c.Limit))
	}
	if c.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", c.Offset))
	}
	if len(q) > 0 {
		endpoint.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ListRunsResult{}, fmt.Errorf("list runs: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return ListRunsResult{}, fmt.Errorf("list runs: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return ListRunsResult{}, httpx.WrapError("list runs", resp.Status, resp.Body)
	}

	var result ListRunsResult
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return ListRunsResult{}, fmt.Errorf("list runs: decode response: %w", err)
	}

	return result, nil
}
