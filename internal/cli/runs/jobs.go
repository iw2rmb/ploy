package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// RepoJobEntry represents a job within a repo execution.
type RepoJobEntry struct {
	JobID       domaintypes.JobID   `json:"job_id"`
	Name        string              `json:"name"`
	JobType     string              `json:"job_type"`
	JobImage    string              `json:"job_image"`
	NextID      *domaintypes.JobID  `json:"next_id"`
	NodeID      *domaintypes.NodeID `json:"node_id"`
	Status      store.JobStatus     `json:"status"`
	StartedAt   *time.Time          `json:"started_at,omitempty"`
	FinishedAt  *time.Time          `json:"finished_at,omitempty"`
	DurationMs  int64               `json:"duration_ms"`
	DisplayName string              `json:"display_name,omitempty"`
}

// ListRepoJobsResult contains the response from listing repo jobs.
type ListRepoJobsResult struct {
	RunID   domaintypes.RunID     `json:"run_id"`
	RepoID  domaintypes.MigRepoID `json:"repo_id"`
	Attempt int32                 `json:"attempt"`
	Jobs    []RepoJobEntry        `json:"jobs"`
}

// ListRepoJobsCommand fetches jobs for a repo execution.
type ListRepoJobsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	RepoID  domaintypes.MigRepoID
	Attempt *int32 // Optional: specific attempt
}

// Run executes GET /v1/runs/{run_id}/repos/{repo_id}/jobs.
func (c ListRepoJobsCommand) Run(ctx context.Context) (ListRepoJobsResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: %w", err)
	}
	if c.RunID.IsZero() {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: run id required")
	}
	if c.RepoID.IsZero() {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: repo id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "repos", c.RepoID.String(), "jobs")
	if c.Attempt != nil {
		q := endpoint.Query()
		q.Set("attempt", fmt.Sprintf("%d", *c.Attempt))
		endpoint.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return ListRepoJobsResult{}, httpx.WrapError("list repo jobs", resp.Status, resp.Body)
	}

	var result ListRepoJobsResult
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: decode response: %w", err)
	}

	result.Jobs = orderRepoJobsByChain(result.Jobs)

	return result, nil
}

// orderRepoJobsByChain reconstructs execution order from linked next_id pointers.
// Head jobs are derived as jobs that have no predecessor in the same payload.
func orderRepoJobsByChain(jobs []RepoJobEntry) []RepoJobEntry {
	if len(jobs) <= 1 {
		return jobs
	}

	jobByID := make(map[domaintypes.JobID]RepoJobEntry, len(jobs))
	orderedIDs := make([]domaintypes.JobID, 0, len(jobs))
	predecessors := make(map[domaintypes.JobID]int, len(jobs))
	nextByID := make(map[domaintypes.JobID]domaintypes.JobID, len(jobs))

	for _, job := range jobs {
		jobByID[job.JobID] = job
		orderedIDs = append(orderedIDs, job.JobID)
		predecessors[job.JobID] = 0
	}

	for _, job := range jobs {
		if job.NextID == nil || job.NextID.IsZero() {
			continue
		}
		nextID := *job.NextID
		if _, ok := jobByID[nextID]; !ok {
			continue
		}
		predecessors[nextID]++
		nextByID[job.JobID] = nextID
	}

	heads := make([]domaintypes.JobID, 0, len(jobs))
	for _, id := range orderedIDs {
		if predecessors[id] == 0 {
			heads = append(heads, id)
		}
	}
	sortJobIDs(heads)

	out := make([]RepoJobEntry, 0, len(jobs))
	visited := make(map[domaintypes.JobID]struct{}, len(jobs))
	walkChain := func(start domaintypes.JobID) {
		current := start
		for {
			if _, seen := visited[current]; seen {
				return
			}
			job, ok := jobByID[current]
			if !ok {
				return
			}

			visited[current] = struct{}{}
			out = append(out, job)

			nextID, ok := nextByID[current]
			if !ok {
				return
			}
			current = nextID
		}
	}

	for _, head := range heads {
		walkChain(head)
	}

	if len(out) == len(jobs) {
		return out
	}

	remaining := make([]domaintypes.JobID, 0, len(jobs)-len(out))
	for _, id := range orderedIDs {
		if _, ok := visited[id]; !ok {
			remaining = append(remaining, id)
		}
	}
	sortJobIDs(remaining)
	for _, id := range remaining {
		walkChain(id)
	}

	return out
}

func sortJobIDs(ids []domaintypes.JobID) {
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
}
