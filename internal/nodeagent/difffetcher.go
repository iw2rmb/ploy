package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// healBranchPattern matches healing job names with branch identifiers.
// Format: heal-{branch_name}-{attempt}-{mod_index} or re-gate-{branch_name}-{attempt}
// Examples: "heal-branch-a-1-0", "heal-codex-ai-1-0", "re-gate-branch-a-1".
var healBranchPattern = regexp.MustCompile(`^(?:heal|re-gate)-(.+?)-\d+(?:-\d+)?$`)

// ExtractBranchFromJobName extracts the branch name from a healing job name.
// Returns the branch name if the job is part of a branch-local healing strategy,
// or empty string for mainline jobs (non-healing or non-branch names).
func ExtractBranchFromJobName(jobName string) string {
	matches := healBranchPattern.FindStringSubmatch(jobName)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// DiffFetcher fetches diffs from the control-plane server.
// This is the symmetric counterpart to DiffUploader, enabling nodes to download
// gzipped patches for workspace rehydration during multi-step Mods runs.
//
// C2: The fetcher lists all diffs (mod + healing) for a run; rehydration callers
// then select non-healing diffs (mod_type!="healing") with step_index <= k to
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
	ID        string          `json:"id"`
	JobID     string          `json:"job_id"`
	StepIndex types.StepIndex `json:"step_index"`
	CreatedAt time.Time       `json:"created_at"`
	Size      int             `json:"gzipped_size"`
	Summary   any             `json:"summary,omitempty"` // DiffSummary is map[string]any
}

// diffListResponse is the response structure for listing diffs.
type diffListResponse struct {
	Diffs []diffListItem `json:"diffs"`
}

// ListRunDiffs fetches the list of diffs for a given run (ticket) from the control plane.
// Returns the list of diff metadata items ordered by step_index, then created_at (as per the API).
//
// GET /v1/mods/{id}/diffs
func (f *DiffFetcher) ListRunDiffs(ctx context.Context, runID string) ([]diffListItem, error) {
	url := fmt.Sprintf("%s/v1/mods/%s/diffs", f.cfg.ServerURL, runID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list diffs failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result diffListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Diffs, nil
}

// FetchDiffPatch downloads the gzipped patch for a given diff ID from the control plane.
// Returns the raw gzipped patch bytes ready for decompression and application.
//
// GET /v1/diffs/{id}?download=true
func (f *DiffFetcher) FetchDiffPatch(ctx context.Context, diffID string) ([]byte, error) {
	url := fmt.Sprintf("%s/v1/diffs/%s?download=true", f.cfg.ServerURL, diffID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch diff patch failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read the entire gzipped patch into memory.
	// Diffs are capped at 1 MiB gzipped by the uploader, so this is safe.
	patchBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read patch bytes: %w", err)
	}

	return patchBytes, nil
}

// FetchDiffsForStep fetches all gzipped patches for non-healing diffs up to (and including)
// the specified step index. This combines ListRunDiffs and FetchDiffPatch to retrieve the
// ordered set of patches needed to rehydrate a workspace for step k+1.
//
// C2: Healing diffs (mod_type="healing") share the same step_index as their parent mod step
// for observability, but rehydration uses only non-healing diffs (mod_type!="healing").
// Each per-step mod diff is incremental from the rehydrated baseline, so applying only
// these diffs in step_index order reconstructs the workspace safely.
func (f *DiffFetcher) FetchDiffsForStep(ctx context.Context, runID string, stepIndex types.StepIndex) ([][]byte, error) {
	// Delegate to branch-aware method with empty branch (mainline behavior).
	return f.FetchDiffsForBranch(ctx, runID, stepIndex, "")
}

// FetchDiffsForBranch fetches gzipped patches for workspace rehydration with branch-local isolation.
// This is the core E3 implementation that ensures branch workspaces are isolated.
//
// Branch isolation rules:
//   - Mainline diffs (branch_id="" or absent) are included for all branches.
//   - Branch-specific diffs (branch_id="branch-a") are only included when targetBranch matches.
//   - If targetBranch is empty, only mainline diffs are included (legacy single-branch behavior).
//
// This ensures that parallel healing branches (e.g., branch-a, branch-b) don't accidentally
// apply each other's diffs during rehydration, keeping workspaces isolated per branch.
//
// Example workspace construction:
//   - workspace_branch_a = base + mainline_diffs + diffs_branch_a
//   - workspace_branch_b = base + mainline_diffs + diffs_branch_b
func (f *DiffFetcher) FetchDiffsForBranch(ctx context.Context, runID string, stepIndex types.StepIndex, targetBranch string) ([][]byte, error) {
	// Step 1: List all diffs for the run.
	diffs, err := f.ListRunDiffs(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("list diffs: %w", err)
	}

	// Step 2: Filter diffs up to the target step index (inclusive).
	// Apply branch-local isolation: only include mainline diffs and same-branch diffs.
	var relevantDiffs []diffListItem
	for _, d := range diffs {
		if d.StepIndex > stepIndex {
			continue
		}

		summary, _ := d.Summary.(map[string]any)

		// Skip healing diffs in the patch chain. Healing diffs share the same step_index
		// for telemetry but represent intermediate workspace states that are already
		// captured in the final per-step mod diff.
		if summary != nil {
			if modType, ok := summary["mod_type"].(string); ok && modType == "healing" {
				continue
			}
		}

		// E3: Branch-local isolation — filter by branch_id in diff summary.
		// Include diff if:
		//   1. Diff has no branch_id (mainline) → always included.
		//   2. Diff has branch_id AND targetBranch matches → included (same branch).
		//   3. Diff has branch_id AND targetBranch is empty → excluded (mainline-only mode).
		//   4. Diff has branch_id AND targetBranch differs → excluded (different branch).
		diffBranch := ""
		if summary != nil {
			if b, ok := summary["branch_id"].(string); ok {
				diffBranch = b
			}
		}

		// Mainline diffs (no branch_id) are always included.
		// Branch diffs are only included if targetBranch matches.
		if diffBranch != "" {
			// Diff belongs to a branch. Only include if caller's branch matches.
			if targetBranch == "" || diffBranch != targetBranch {
				continue
			}
		}

		relevantDiffs = append(relevantDiffs, d)
	}

	// Step 3: Fetch each diff's gzipped patch in order.
	patches := make([][]byte, 0, len(relevantDiffs))
	for _, d := range relevantDiffs {
		patch, err := f.FetchDiffPatch(ctx, d.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch patch for diff %s (step %.0f): %w", d.ID, d.StepIndex, err)
		}
		patches = append(patches, patch)
	}

	return patches, nil
}
