package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

const migListPageSize = 100

// GetRunReportCommand builds the canonical RunReport payload for a run.
type GetRunReportCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
}

// Run assembles run summary, mig identity, repo rows, job rows, and links.
func (c GetRunReportCommand) Run(ctx context.Context) (RunReport, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return RunReport{}, fmt.Errorf("run report: %w", err)
	}
	if c.RunID.IsZero() {
		return RunReport{}, fmt.Errorf("run report: run id required")
	}

	summary, err := GetStatusCommand(c).Run(ctx)
	if err != nil {
		return RunReport{}, err
	}

	migName, err := resolveMigNameByID(ctx, c.Client, c.BaseURL, summary.MigID)
	if err != nil {
		return RunReport{}, err
	}

	repos, err := listRunRepos(ctx, c.Client, c.BaseURL, c.RunID)
	if err != nil {
		return RunReport{}, err
	}

	report := RunReport{
		RunID:   summary.ID,
		MigID:   summary.MigID,
		MigName: migName,
		SpecID:  summary.SpecID,
		Repos:   make([]RepoReport, 0, len(repos)),
		Runs:    make([]RunEntry, 0, len(repos)),
	}

	for _, repo := range repos {
		jobsResult, err := ListRepoJobsCommand{
			Client:  c.Client,
			BaseURL: c.BaseURL,
			RunID:   c.RunID,
			RepoID:  repo.RepoID,
			Attempt: &repo.Attempt,
		}.Run(ctx)
		if err != nil {
			return RunReport{}, fmt.Errorf("run report: list repo jobs (%s): %w", repo.RepoID, err)
		}

		diffs, err := listRunRepoDiffs(ctx, c.Client, c.BaseURL, c.RunID, repo.RepoID)
		if err != nil {
			return RunReport{}, fmt.Errorf("run report: list repo diffs (%s): %w", repo.RepoID, err)
		}

		buildLogURL := buildRepoLogURL(c.BaseURL, c.RunID, repo.RepoID)
		repoPatchURL := ""
		if latest := latestRepoDiff(diffs); latest != nil {
			repoPatchURL = buildRepoPatchURL(c.BaseURL, c.RunID, repo.RepoID, latest.ID)
		}

		jobPatchByID := make(map[domaintypes.JobID]string, len(diffs))
		for _, diff := range diffs {
			if diff.JobID.IsZero() {
				continue
			}
			jobPatchByID[diff.JobID] = buildRepoPatchURL(c.BaseURL, c.RunID, repo.RepoID, diff.ID)
		}

		report.Repos = append(report.Repos, RepoReport{
			RepoID:      repo.RepoID,
			RepoURL:     repo.RepoURL,
			BaseRef:     repo.BaseRef,
			TargetRef:   repo.TargetRef,
			Status:      repo.Status,
			Attempt:     repo.Attempt,
			LastError:   repo.LastError,
			BuildLogURL: buildLogURL,
			PatchURL:    repoPatchURL,
		})

		runEntry := RunEntry{
			RepoID:      repo.RepoID,
			RepoURL:     repo.RepoURL,
			BaseRef:     repo.BaseRef,
			TargetRef:   repo.TargetRef,
			Attempt:     repo.Attempt,
			Status:      repo.Status,
			LastError:   repo.LastError,
			BuildLogURL: buildLogURL,
			PatchURL:    repoPatchURL,
			Jobs:        make([]RunJobEntry, 0, len(jobsResult.Jobs)),
		}

		for _, job := range jobsResult.Jobs {
			runEntry.Jobs = append(runEntry.Jobs, RunJobEntry{
				JobID:         job.JobID,
				JobType:       job.JobType,
				JobImage:      job.JobImage,
				NodeID:        job.NodeID,
				Status:        string(job.Status),
				ExitCode:      job.ExitCode,
				StartedAt:     job.StartedAt,
				FinishedAt:    job.FinishedAt,
				DurationMs:    job.DurationMs,
				DisplayName:   job.DisplayName,
				ActionSummary: job.ActionSummary,
				BugSummary:    job.BugSummary,
				Recovery:      job.Recovery,
				BuildLogURL:   buildLogURL,
				PatchURL:      jobPatchByID[job.JobID],
			})
		}

		report.Runs = append(report.Runs, runEntry)
	}

	return report, nil
}

type runRepoReportSource struct {
	RunID      domaintypes.RunID     `json:"run_id"`
	RepoID     domaintypes.MigRepoID `json:"repo_id"`
	RepoURL    string                `json:"repo_url"`
	BaseRef    string                `json:"base_ref"`
	TargetRef  string                `json:"target_ref"`
	Status     string                `json:"status"`
	Attempt    int32                 `json:"attempt"`
	LastError  *string               `json:"last_error,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	StartedAt  *time.Time            `json:"started_at,omitempty"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
}

func listRunRepos(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID) ([]runRepoReportSource, error) {
	endpoint := baseURL.JoinPath("v1", "runs", runID.String(), "repos")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("run report: build run repos request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("run report: fetch run repos failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("run report: fetch run repos", resp.Status, resp.Body)
	}

	var result struct {
		Repos []runRepoReportSource `json:"repos"`
	}
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run report: decode run repos: %w", err)
	}
	if result.Repos == nil {
		result.Repos = make([]runRepoReportSource, 0)
	}

	return result.Repos, nil
}

type migListItem struct {
	ID        domaintypes.MigID   `json:"id"`
	Name      string              `json:"name"`
	SpecID    *domaintypes.SpecID `json:"spec_id,omitempty"`
	CreatedBy *string             `json:"created_by,omitempty"`
	Archived  bool                `json:"archived"`
	CreatedAt time.Time           `json:"created_at"`
}

func resolveMigNameByID(ctx context.Context, httpClient *http.Client, baseURL *url.URL, migID domaintypes.MigID) (string, error) {
	if migID.IsZero() {
		return "", nil
	}

	offset := 0
	for {
		page, err := listMigsPage(ctx, httpClient, baseURL, migListPageSize, offset)
		if err != nil {
			return "", err
		}
		for _, mig := range page {
			if mig.ID == migID {
				return mig.Name, nil
			}
		}
		if len(page) < migListPageSize {
			return "", nil
		}
		offset += migListPageSize
	}
}

func listMigsPage(ctx context.Context, httpClient *http.Client, baseURL *url.URL, limit int, offset int) ([]migListItem, error) {
	endpoint := baseURL.JoinPath("v1", "migs")
	q := endpoint.Query()
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("run report: build migs request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("run report: fetch migs failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("run report: fetch migs", resp.Status, resp.Body)
	}

	var result struct {
		Migs []migListItem `json:"migs"`
	}
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run report: decode migs: %w", err)
	}
	if result.Migs == nil {
		result.Migs = make([]migListItem, 0)
	}

	return result.Migs, nil
}

type repoDiffEntry struct {
	ID        domaintypes.DiffID      `json:"id"`
	JobID     domaintypes.JobID       `json:"job_id"`
	CreatedAt time.Time               `json:"created_at"`
	Size      int                     `json:"gzipped_size"`
	Summary   domaintypes.DiffSummary `json:"summary,omitempty"`
}

func listRunRepoDiffs(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID, repoID domaintypes.MigRepoID) ([]repoDiffEntry, error) {
	endpoint := baseURL.JoinPath("v1", "runs", runID.String(), "repos", repoID.String(), "diffs")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("run report: build diffs request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("run report: fetch diffs failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("run report: fetch diffs", resp.Status, resp.Body)
	}

	var result struct {
		Diffs []repoDiffEntry `json:"diffs"`
	}
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run report: decode diffs: %w", err)
	}
	if result.Diffs == nil {
		result.Diffs = make([]repoDiffEntry, 0)
	}

	return result.Diffs, nil
}

func latestRepoDiff(diffs []repoDiffEntry) *repoDiffEntry {
	if len(diffs) == 0 {
		return nil
	}
	latest := diffs[len(diffs)-1]
	return &latest
}

func buildRepoLogURL(baseURL *url.URL, runID domaintypes.RunID, repoID domaintypes.MigRepoID) string {
	return baseURL.JoinPath("v1", "runs", runID.String(), "repos", repoID.String(), "logs").String()
}

func buildRepoPatchURL(baseURL *url.URL, runID domaintypes.RunID, repoID domaintypes.MigRepoID, diffID domaintypes.DiffID) string {
	u := baseURL.JoinPath("v1", "runs", runID.String(), "repos", repoID.String(), "diffs")
	q := u.Query()
	q.Set("download", "true")
	q.Set("diff_id", diffID.String())
	u.RawQuery = q.Encode()
	return u.String()
}
