package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// DiffFetcher fetches diffs from the control-plane server.
// This is the symmetric counterpart to DiffUploader, enabling nodes to download
// gzipped patches for workspace rehydration during multi-step Mods runs.
//
// The fetcher lists all diffs for a run/repo and lets callers apply deterministic
// ordering rules for rehydration.
type DiffFetcher struct {
	*baseUploader
}

// NewDiffFetcher creates a new diff fetcher.
func NewDiffFetcher(cfg Config) (*DiffFetcher, error) {
	base, err := newBaseUploader(cfg)
	if err != nil {
		return nil, err
	}
	return &DiffFetcher{baseUploader: base}, nil
}

// diffListItem represents a single diff in the list response from the control plane.
type diffListItem struct {
	ID        string            `json:"id"`
	JobID     types.JobID       `json:"job_id"`
	CreatedAt time.Time         `json:"created_at"`
	Size      int               `json:"gzipped_size"`
	Summary   types.DiffSummary `json:"summary,omitempty"`
}

type diffListResponse struct {
	Diffs []diffListItem `json:"diffs"`
}

// ListRunRepoDiffs fetches the list of diffs for a specific repo within a run.
func (f *DiffFetcher) ListRunRepoDiffs(ctx context.Context, runID types.RunID, repoID types.MigRepoID) ([]diffListItem, error) {
	apiPath := fmt.Sprintf("/v1/runs/%s/repos/%s/diffs", runID.String(), repoID.String())
	url := MustBuildURL(f.cfg.ServerURL, apiPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := httpError(resp, http.StatusOK, "list run repo diffs"); err != nil {
		return nil, err
	}

	var result diffListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Diffs, nil
}

// FetchDiffsForJobRepo fetches all prior gzipped patches for a specific repo within
// a run, excluding the current job's own diff.
func (f *DiffFetcher) FetchDiffsForJobRepo(ctx context.Context, runID types.RunID, repoID types.MigRepoID, currentJobID types.JobID) ([][]byte, error) {
	diffs, err := f.ListRunRepoDiffs(ctx, runID, repoID)
	if err != nil {
		return nil, fmt.Errorf("list run repo diffs: %w", err)
	}

	var relevantDiffs []diffListItem
	for _, d := range diffs {
		if !currentJobID.IsZero() && d.JobID == currentJobID {
			continue
		}

		// Skip legacy healing-tagged diffs; discrete healing jobs are tagged as "mig".
		if d.Summary.JobType() == DiffJobTypeHealing.String() {
			continue
		}

		relevantDiffs = append(relevantDiffs, d)
	}

	// Sort by (created_at, id) for deterministic patch application order without
	// next_id dependence.
	sort.SliceStable(relevantDiffs, func(i, j int) bool {
		if !relevantDiffs[i].CreatedAt.Equal(relevantDiffs[j].CreatedAt) {
			return relevantDiffs[i].CreatedAt.Before(relevantDiffs[j].CreatedAt)
		}
		return relevantDiffs[i].ID < relevantDiffs[j].ID
	})

	patches := make([][]byte, 0, len(relevantDiffs))
	for _, d := range relevantDiffs {
		patch, err := f.FetchRunRepoDiffPatch(ctx, runID, repoID, d.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch patch for diff %s: %w", d.ID, err)
		}
		patches = append(patches, patch)
	}

	return patches, nil
}

// FetchRunRepoDiffPatch downloads the gzipped patch for a specific diff.
func (f *DiffFetcher) FetchRunRepoDiffPatch(ctx context.Context, runID types.RunID, repoID types.MigRepoID, diffID string) ([]byte, error) {
	base, err := url.Parse(f.cfg.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("parse server url: %w", err)
	}
	endpoint := base.JoinPath("v1", "runs", runID.String(), "repos", repoID.String(), "diffs")
	q := endpoint.Query()
	q.Set("download", "true")
	q.Set("diff_id", diffID)
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := httpError(resp, http.StatusOK, "fetch run repo diff patch"); err != nil {
		return nil, err
	}

	patchBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read patch bytes: %w", err)
	}

	return patchBytes, nil
}
