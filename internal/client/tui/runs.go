package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
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

	return httpx.DoJSON[ListRunsResult](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "list runs")
}
