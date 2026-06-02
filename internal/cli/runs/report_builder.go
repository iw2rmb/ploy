package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// GetRunStatusReportCommand builds the canonical RunStatusReport payload for a run.
type GetRunStatusReportCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
}

// Run assembles run summary, mig identity, run job rows, and links.
func (c GetRunStatusReportCommand) Run(ctx context.Context) (RunStatusReport, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return RunStatusReport{}, fmt.Errorf("run status report: %w", err)
	}
	if c.RunID.IsZero() {
		return RunStatusReport{}, fmt.Errorf("run status report: run id required")
	}

	summary, err := GetStatusCommand(c).Run(ctx)
	if err != nil {
		return RunStatusReport{}, err
	}

	stageArtifacts, err := listRunStageArtifacts(ctx, c.Client, c.BaseURL, c.RunID)
	if err != nil {
		return RunStatusReport{}, err
	}

	report := RunStatusReport{
		RunID:   summary.ID,
		MigID:   summary.MigID,
		MigName: summary.MigName,
		SpecID:  summary.SpecID,
		Repos:   make([]RunEntry, 1),
	}

	if err := c.buildRunEntry(ctx, statusReportSourceFromSummary(c.RunID, summary), stageArtifacts, &report.Repos[0]); err != nil {
		return RunStatusReport{}, err
	}
	if hasSuccessfulPostGate(report.Repos[0].Jobs) {
		sbom, err := GetRunSBOMCommand{
			Client:  c.Client,
			BaseURL: c.BaseURL,
			RunID:   c.RunID,
			View:    "diff",
		}.Run(ctx)
		if err != nil {
			return RunStatusReport{}, fmt.Errorf("run status report: fetch sbom diff: %w", err)
		}
		report.SBOMDiff = sbom.DiffPackages
	}

	return report, nil
}

func (c GetRunStatusReportCommand) buildRunEntry(
	ctx context.Context,
	run statusReportSource,
	stageArtifacts map[domaintypes.JobID]map[string]string,
	out *RunEntry,
) error {
	jobsResult, err := ListRunJobsCommand{
		Client:  c.Client,
		BaseURL: c.BaseURL,
		RunID:   c.RunID,
		Attempt: &run.Attempt,
	}.Run(ctx)
	if err != nil {
		return fmt.Errorf("run status report: list run jobs: %w", err)
	}

	diffs, err := listRunDiffs(ctx, c.Client, c.BaseURL, c.RunID)
	if err != nil {
		return fmt.Errorf("run status report: list run diffs: %w", err)
	}

	repoPatchURL := ""
	if latest := latestRunDiff(diffs); latest != nil {
		repoPatchURL = buildRunPatchURL(c.BaseURL, c.RunID, latest.ID, true)
	}

	jobPatchByID := make(map[domaintypes.JobID]string, len(diffs))
	for _, diff := range diffs {
		if diff.JobID.IsZero() {
			continue
		}
		jobPatchByID[diff.JobID] = buildRunPatchURL(c.BaseURL, c.RunID, diff.ID, true)
	}

	*out = RunEntry{
		RepoID:          run.RepoID,
		RepoURL:         run.RepoURL,
		BaseRef:         run.BaseRef,
		SourceCommitSHA: run.SourceCommitSHA,
		Attempt:         run.Attempt,
		Status:          run.Status,
		LastError:       run.LastError,
		PatchURL:        repoPatchURL,
		Jobs:            make([]RunJobEntry, 0, len(jobsResult.Jobs)),
	}

	for _, job := range jobsResult.Jobs {
		out.Jobs = append(out.Jobs, RunJobEntry{
			JobID:       job.JobID,
			JobType:     job.JobType,
			JobImage:    job.JobImage,
			NodeID:      job.NodeID,
			Status:      job.Status,
			ExitCode:    job.ExitCode,
			StartedAt:   job.StartedAt,
			FinishedAt:  job.FinishedAt,
			DurationMs:  job.DurationMs,
			DisplayName: job.DisplayName,
			BugSummary:  job.BugSummary,
			Artifacts:   buildJobArtifacts(c.BaseURL, stageArtifacts[job.JobID]),
			JobLogURL:   buildJobLogURL(c.BaseURL, job.JobID),
			PatchURL:    jobPatchByID[job.JobID],
		})
	}

	return nil
}

type statusReportSource struct {
	RunID           domaintypes.RunID  `json:"run_id"`
	RepoID          domaintypes.RepoID `json:"repo_id"`
	RepoURL         string             `json:"repo_url"`
	BaseRef         string             `json:"base_ref"`
	SourceCommitSHA string             `json:"source_commit_sha,omitempty"`
	Status          domaintypes.RunStatus
	Attempt         int32
	LastError       *string
}

func statusReportSourceFromSummary(runID domaintypes.RunID, summary domaintypes.RunSummary) statusReportSource {
	return statusReportSource{
		RunID:           runID,
		RepoID:          summary.RepoID,
		RepoURL:         summary.RepoURL,
		BaseRef:         summary.BaseRef,
		SourceCommitSHA: summary.SourceCommitSHA,
		Status:          summary.Status,
		Attempt:         summary.Attempt,
		LastError:       summary.LastError,
	}
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
		return nil, fmt.Errorf("run status report: build run stage artifacts request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("run status report: fetch run stage artifacts failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("run status report: fetch run stage artifacts", resp.Status, resp.Body)
	}

	var summary migsapi.RunSummary
	if err := httpx.DecodeResponseJSON(resp.Body, &summary, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run status report: decode run stage artifacts: %w", err)
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

func listRunDiffs(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID) ([]RunDiffEntry, error) {
	endpoint := baseURL.JoinPath("v1", "runs", runID.String(), "diffs")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("run status report: build diffs request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("run status report: fetch diffs failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("run status report: fetch diffs", resp.Status, resp.Body)
	}

	var result struct {
		Diffs []RunDiffEntry `json:"diffs"`
	}
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run status report: decode diffs: %w", err)
	}
	if result.Diffs == nil {
		result.Diffs = make([]RunDiffEntry, 0)
	}

	return result.Diffs, nil
}

// latestRunDiff returns the most recent diff entry.
// The API returns diffs ordered by created_at ascending, so the last element is the latest.
func latestRunDiff(diffs []RunDiffEntry) *RunDiffEntry {
	if len(diffs) == 0 {
		return nil
	}
	latest := diffs[len(diffs)-1]
	return &latest
}

func buildJobLogURL(baseURL *url.URL, jobID domaintypes.JobID) string {
	if baseURL == nil || jobID.IsZero() {
		return ""
	}
	return baseURL.JoinPath("v1", "jobs", jobID.String(), "logs").String()
}

func buildRunPatchURL(baseURL *url.URL, runID domaintypes.RunID, diffID domaintypes.DiffID, accumulated bool) string {
	u := baseURL.JoinPath("v1", "runs", runID.String(), "diffs")
	q := u.Query()
	q.Set("download", "true")
	q.Set("diff_id", diffID.String())
	if accumulated {
		q.Set("accumulated", "true")
	}
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

func hasSuccessfulPostGate(jobs []RunJobEntry) bool {
	for _, job := range jobs {
		if job.JobType == domaintypes.JobTypePostGate && isSuccessfulStatus(job.Status.String()) {
			return true
		}
	}
	return false
}
