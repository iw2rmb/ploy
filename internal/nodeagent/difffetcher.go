package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DiffFetcher fetches diffs from the control-plane server.
// This is the symmetric counterpart to DiffUploader, enabling nodes to download
// gzipped patches for workspace rehydration during multi-step Mods runs.
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
	ID        string    `json:"id"`
	StageID   string    `json:"stage_id"`
	StepIndex *int32    `json:"step_index,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Size      int       `json:"gzipped_size"`
	Summary   any       `json:"summary,omitempty"` // DiffSummary is map[string]any
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

// FetchDiffsForStep fetches all gzipped patches for diffs up to (and including) the specified step index.
// This is a convenience helper that combines ListRunDiffs and FetchDiffPatch to retrieve
// the ordered set of patches needed to rehydrate a workspace for step k+1.
//
// The returned slice contains gzipped patches in step order (step 0, 1, ..., stepIndex).
// Only diffs with non-nil step_index values are included (legacy aggregate diffs are excluded).
func (f *DiffFetcher) FetchDiffsForStep(ctx context.Context, runID string, stepIndex int32) ([][]byte, error) {
	// Step 1: List all diffs for the run.
	diffs, err := f.ListRunDiffs(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("list diffs: %w", err)
	}

	// Step 2: Filter diffs up to the target step index (inclusive).
	// Only include diffs with non-nil step_index (exclude legacy/aggregate diffs).
	var relevantDiffs []diffListItem
	for _, d := range diffs {
		if d.StepIndex != nil && *d.StepIndex <= stepIndex {
			relevantDiffs = append(relevantDiffs, d)
		}
	}

	// Step 3: Fetch each diff's gzipped patch in order.
	patches := make([][]byte, 0, len(relevantDiffs))
	for _, d := range relevantDiffs {
		patch, err := f.FetchDiffPatch(ctx, d.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch patch for diff %s (step %d): %w", d.ID, *d.StepIndex, err)
		}
		patches = append(patches, patch)
	}

	return patches, nil
}
