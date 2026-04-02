package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

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

	repos, err := listRunRepos(ctx, c.Client, c.BaseURL, c.RunID)
	if err != nil {
		return RunReport{}, err
	}
	stageArtifacts, err := listRunStageArtifacts(ctx, c.Client, c.BaseURL, c.RunID)
	if err != nil {
		return RunReport{}, err
	}

	report := RunReport{
		RunID:   summary.ID,
		MigID:   summary.MigID,
		MigName: summary.MigName,
		SpecID:  summary.SpecID,
		Repos:   make([]RunEntry, len(repos)),
	}

	g, gctx := errgroup.WithContext(ctx)
	for i, repo := range repos {
		g.Go(func() error {
			return c.buildRepoEntry(gctx, repo, stageArtifacts, &report.Repos[i])
		})
	}
	if err := g.Wait(); err != nil {
		return RunReport{}, err
	}

	return report, nil
}

func (c GetRunReportCommand) buildRepoEntry(
	ctx context.Context,
	repo runRepoReportSource,
	stageArtifacts map[domaintypes.JobID]map[string]string,
	out *RunEntry,
) error {
	jobsResult, err := ListRepoJobsCommand{
		Client:  c.Client,
		BaseURL: c.BaseURL,
		RunID:   c.RunID,
		RepoID:  repo.RepoID,
		Attempt: &repo.Attempt,
	}.Run(ctx)
	if err != nil {
		return fmt.Errorf("run report: list repo jobs (%s): %w", repo.RepoID, err)
	}

	diffs, err := listRunRepoDiffs(ctx, c.Client, c.BaseURL, c.RunID, repo.RepoID)
	if err != nil {
		return fmt.Errorf("run report: list repo diffs (%s): %w", repo.RepoID, err)
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

	*out = RunEntry{
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
		out.Jobs = append(out.Jobs, RunJobEntry{
			JobID:         job.JobID,
			JobType:       job.JobType,
			JobImage:      job.JobImage,
			NodeID:        job.NodeID,
			Status:        job.Status,
			ExitCode:      job.ExitCode,
			StartedAt:     job.StartedAt,
			FinishedAt:    job.FinishedAt,
			DurationMs:    job.DurationMs,
			DisplayName:   job.DisplayName,
			ActionSummary: job.ActionSummary,
			BugSummary:    job.BugSummary,
			Recovery:      job.Recovery,
			Artifacts:     buildJobArtifacts(c.BaseURL, stageArtifacts[job.JobID]),
			BuildLogURL:   buildLogURL,
			PatchURL:      jobPatchByID[job.JobID],
		})
	}

	return nil
}

type runRepoReportSource struct {
	RunID      domaintypes.RunID         `json:"run_id"`
	RepoID     domaintypes.MigRepoID     `json:"repo_id"`
	RepoURL    string                    `json:"repo_url"`
	BaseRef    string                    `json:"base_ref"`
	TargetRef  string                    `json:"target_ref"`
	Status     domaintypes.RunRepoStatus `json:"status"`
	Attempt    int32                     `json:"attempt"`
	LastError  *string                   `json:"last_error,omitempty"`
	CreatedAt  time.Time                 `json:"created_at"`
	StartedAt  *time.Time                `json:"started_at,omitempty"`
	FinishedAt *time.Time                `json:"finished_at,omitempty"`
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
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run report: decode run repos: %w", err)
	}
	if result.Repos == nil {
		result.Repos = make([]runRepoReportSource, 0)
	}

	return result.Repos, nil
}

func listRunStageArtifacts(
	ctx context.Context,
	httpClient *http.Client,
	baseURL *url.URL,
	runID domaintypes.RunID,
) (map[domaintypes.JobID]map[string]string, error) {
	endpoint := baseURL.JoinPath("v1", "runs", runID.String(), "status")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("run report: build run stage artifacts request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("run report: fetch run stage artifacts failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("run report: fetch run stage artifacts", resp.Status, resp.Body)
	}

	var summary migsapi.RunSummary
	if err := httpx.DecodeResponseJSON(resp.Body, &summary, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run report: decode run stage artifacts: %w", err)
	}

	artifacts := make(map[domaintypes.JobID]map[string]string, len(summary.Stages))
	for jobID, stage := range summary.Stages {
		if len(stage.Artifacts) == 0 {
			continue
		}
		copied := make(map[string]string, len(stage.Artifacts))
		for name, cid := range stage.Artifacts {
			name = strings.TrimSpace(name)
			cid = strings.TrimSpace(cid)
			if name == "" || cid == "" {
				continue
			}
			copied[name] = cid
		}
		if len(copied) > 0 {
			artifacts[jobID] = copied
		}
	}
	return artifacts, nil
}

func listRunRepoDiffs(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID, repoID domaintypes.MigRepoID) ([]RepoDiffEntry, error) {
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
		Diffs []RepoDiffEntry `json:"diffs"`
	}
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run report: decode diffs: %w", err)
	}
	if result.Diffs == nil {
		result.Diffs = make([]RepoDiffEntry, 0)
	}

	return result.Diffs, nil
}

// latestRepoDiff returns the most recent diff entry.
// The API returns diffs ordered by created_at ascending, so the last element is the latest.
func latestRepoDiff(diffs []RepoDiffEntry) *RepoDiffEntry {
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

func buildJobArtifacts(baseURL *url.URL, stageArtifacts map[string]string) []RunJobArtifact {
	if len(stageArtifacts) == 0 {
		return nil
	}

	names := make([]string, 0, len(stageArtifacts))
	for name := range stageArtifacts {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]RunJobArtifact, 0, len(names))
	for _, name := range names {
		cid := strings.TrimSpace(stageArtifacts[name])
		if cid == "" {
			continue
		}
		items = append(items, RunJobArtifact{
			Name:      strings.TrimSpace(name),
			CID:       cid,
			LookupURL: buildArtifactLookupURL(baseURL, cid),
		})
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func buildArtifactLookupURL(baseURL *url.URL, cid string) string {
	if baseURL == nil || strings.TrimSpace(cid) == "" {
		return ""
	}
	u := baseURL.JoinPath("v1", "artifacts")
	q := u.Query()
	q.Set("cid", strings.TrimSpace(cid))
	u.RawQuery = q.Encode()
	return u.String()
}
