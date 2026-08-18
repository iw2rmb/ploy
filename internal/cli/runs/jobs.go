package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/workflow/jobchain"
)

type RunJobDetailEntry = migsapi.RunJob

// ListRunJobsResult contains the response from listing run jobs.
type ListRunJobsResult = migsapi.ListRunJobsResponse

// ListRunJobsCommand fetches jobs for a run execution.
type ListRunJobsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	Attempt *int32 // Optional: specific attempt
}

// Run executes GET /v1/runs/{run_id}/jobs.
func (c ListRunJobsCommand) Run(ctx context.Context) (ListRunJobsResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return ListRunJobsResult{}, fmt.Errorf("list run jobs: %w", err)
	}
	if c.RunID.IsZero() {
		return ListRunJobsResult{}, fmt.Errorf("list run jobs: run id required")
	}
	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "jobs")
	if c.Attempt != nil {
		q := endpoint.Query()
		q.Set("attempt", fmt.Sprintf("%d", *c.Attempt))
		endpoint.RawQuery = q.Encode()
	}

	result, err := httpx.DoJSON[ListRunJobsResult](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "list run jobs")
	if err != nil {
		return ListRunJobsResult{}, err
	}

	result.Jobs = orderRunJobsByChain(result.Jobs)

	return result, nil
}

// orderRunJobsByChain reconstructs execution order from linked next_id pointers.
// Head jobs are derived as jobs that have no predecessor in the same payload.
func orderRunJobsByChain(jobs []migsapi.RunJob) []migsapi.RunJob {
	return jobchain.Order(
		jobs,
		func(job migsapi.RunJob) domaintypes.JobID { return job.JobID },
		func(job migsapi.RunJob) *domaintypes.JobID { return job.NextID },
	)
}
