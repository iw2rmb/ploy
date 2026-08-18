package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// ListJobsCommand fetches a paginated list of jobs with an optional run_id filter.
type ListJobsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Limit   int32
	Offset  int32
	RunID   *domaintypes.RunID // Optional: filter jobs to a specific run.
}

// Run executes GET /v1/jobs.
func (c ListJobsCommand) Run(ctx context.Context) (domainapi.JobListResponse, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domainapi.JobListResponse{}, fmt.Errorf("list jobs: %w", err)
	}

	endpoint := c.BaseURL.JoinPath("v1", "jobs")
	q := endpoint.Query()
	if c.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", c.Limit))
	}
	if c.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", c.Offset))
	}
	if c.RunID != nil && !c.RunID.IsZero() {
		q.Set("run_id", c.RunID.String())
	}
	if len(q) > 0 {
		endpoint.RawQuery = q.Encode()
	}

	return httpx.DoJSON[domainapi.JobListResponse](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "list jobs")
}
