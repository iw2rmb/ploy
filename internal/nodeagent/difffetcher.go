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
// C2: The fetcher lists all diffs (mod + healing) for a run; rehydration callers
// then select non-healing diffs (mod_type!=DiffModTypeHealing) with step_index <= k to
// build the incremental patch chain for step k+1.
type DiffFetcher struct {
	cfg    Config
	client *http.Client
}

// NewDiffFetcher creates a new diff fetcher.
func NewDiffFetcher(cfg Config) (*DiffFetcher, error) {
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	return &DiffFetcher{
		cfg:    cfg,
		client: client,
	}, nil
}

// diffListItem represents a single diff in the list response from the control plane.
// This mirrors the diffItem struct from internal/server/handlers/handlers_diffs.go.
type diffListItem struct {
	ID        string            `json:"id"`
	JobID     types.JobID       `json:"job_id"`
	CreatedAt time.Time         `json:"created_at"`
	Size      int               `json:"gzipped_size"`
	Summary   types.DiffSummary `json:"summary,omitempty"`
}

// diffListResponse is the response structure for listing diffs.
type diffListResponse struct {
	Diffs []diffListItem `json:"diffs"`
}

// ListRunRepoDiffs fetches the list of diffs for a specific repo execution within a run.
// This is the v1 repo-scoped diffs listing endpoint.
//
// Diffs are filtered by repo_id via jobs.repo_id join, so diffs for repo A
// are excluded from repo B listing. Response shape is unchanged from legacy endpoint.
//
// GET /v1/runs/{run_id}/repos/{repo_id}/diffs
func (f *DiffFetcher) ListRunRepoDiffs(ctx context.Context, runID types.RunID, repoID types.ModRepoID) ([]diffListItem, error) {
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

// FetchDiffsForStepRepo fetches all gzipped patches for non-healing diffs up to (and including)
// the specified step index for a specific repo execution within a run.
// This is the v1 repo-scoped version of FetchDiffsForStep that uses repo-scoped diffs listing.
func (f *DiffFetcher) FetchDiffsForStepRepo(ctx context.Context, runID types.RunID, repoID types.ModRepoID, stepIndex types.StepIndex) ([][]byte, error) {
	// Step 1: List all diffs for the repo execution.
	diffs, err := f.ListRunRepoDiffs(ctx, runID, repoID)
	if err != nil {
		return nil, fmt.Errorf("list run repo diffs: %w", err)
	}

	// Step 2: Filter diffs up to the target step index (inclusive).
	var relevantDiffs []diffListItem
	for _, d := range diffs {
		si, ok := d.Summary.StepIndex()
		if !ok {
			continue
		}
		if si > stepIndex {
			continue
		}

		// Skip healing diffs in the patch chain.
		// Healing diffs are filtered out during rehydration to avoid applying
		// intermediate healing states. Discrete healing jobs use DiffModTypeMod
		// so their changes are included in the rehydration chain.
		if d.Summary.ModType() == DiffModTypeHealing.String() {
			continue
		}

		relevantDiffs = append(relevantDiffs, d)
	}

	// Step 3: Sort diffs to ensure deterministic application order.
	// Diffs are sorted by (step_index, created_at, id) to guarantee:
	//   - Earlier steps are applied before later steps.
	//   - Within a step, diffs are ordered chronologically by creation time.
	//   - Ties in step_index and created_at are broken by lexicographic ID comparison.
	// This ensures the patch chain is applied identically regardless of server response order.
	sort.SliceStable(relevantDiffs, func(i, j int) bool {
		// Primary sort key: step_index (ascending).
		siI, _ := relevantDiffs[i].Summary.StepIndex()
		siJ, _ := relevantDiffs[j].Summary.StepIndex()
		if siI != siJ {
			return siI < siJ
		}

		// Secondary sort key: created_at (ascending).
		if !relevantDiffs[i].CreatedAt.Equal(relevantDiffs[j].CreatedAt) {
			return relevantDiffs[i].CreatedAt.Before(relevantDiffs[j].CreatedAt)
		}

		// Tertiary sort key: id (lexicographic ascending, for determinism).
		return relevantDiffs[i].ID < relevantDiffs[j].ID
	})

	// Step 4: Fetch each diff's gzipped patch in sorted order.
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

// FetchRunRepoDiffPatch downloads the gzipped patch for a diff that belongs to a specific
// repo execution within a run.
//
// GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>
func (f *DiffFetcher) FetchRunRepoDiffPatch(ctx context.Context, runID types.RunID, repoID types.ModRepoID, diffID string) ([]byte, error) {
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

	// Read the entire gzipped patch into memory.
	// Diffs are capped at 1 MiB gzipped by the uploader, so this is safe.
	patchBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read patch bytes: %w", err)
	}

	return patchBytes, nil
}
