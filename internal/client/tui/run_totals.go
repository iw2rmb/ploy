package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// RunTotals holds aggregated repo and job counts for a run.
type RunTotals struct {
	RepoTotal int32
	JobTotal  int64
}

// GetRunTotalsCommand fetches repo and job totals for a specific run.
// It calls GET /v1/runs/{id} for repo counts and GET /v1/jobs?run_id={id} for job total.
type GetRunTotalsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
}

// Run fetches the run summary and job count for the configured run.
func (c GetRunTotalsCommand) Run(ctx context.Context) (RunTotals, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return RunTotals{}, fmt.Errorf("get run totals: %w", err)
	}
	if c.RunID.IsZero() {
		return RunTotals{}, fmt.Errorf("get run totals: run id required")
	}

	// Fetch run summary to get repo counts.
	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String())
	summary, err := httpx.DoJSON[domaintypes.RunSummary](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "get run totals")
	if err != nil {
		return RunTotals{}, err
	}

	var repoTotal int32
	if summary.Counts != nil {
		repoTotal = summary.Counts.Total
	}

	// Fetch job count for this run.
	runID := c.RunID
	jobsResult, err := ListJobsCommand{
		Client:  c.Client,
		BaseURL: c.BaseURL,
		Limit:   1,
		RunID:   &runID,
	}.Run(ctx)
	if err != nil {
		return RunTotals{}, fmt.Errorf("get run totals: %w", err)
	}

	return RunTotals{
		RepoTotal: repoTotal,
		JobTotal:  jobsResult.Total,
	}, nil
}
