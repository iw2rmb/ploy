// status.go provides CLI client implementations for fetching diffs.
//
// This file implements helpers used by `ploy mod run pull`:
//   - ListAllDiffsCommand: Fetches all diffs for a run via GET /v1/mods/{id}/diffs (legacy).
//   - ListRunRepoDiffsCommand: Fetches repo-scoped diffs via GET /v1/runs/{run_id}/repos/{repo_id}/diffs (v1).
//   - DownloadDiffCommand: Downloads a single diff via GET /v1/diffs/{diff_id}?download=true.
package mods

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// DiffEntry represents a single diff record from the list diffs response.
type DiffEntry struct {
	ID        string            `json:"id"`
	JobID     domaintypes.JobID `json:"job_id"`
	CreatedAt string            `json:"created_at"`
	Size      int               `json:"gzipped_size"`
	StepIndex int               `json:"step_index"` // Job step index for ordering
}

// ListAllDiffsCommand fetches all diffs for a run via GET /v1/mods/{id}/diffs.
// Returns the list of diff entries sorted by step_index for correct application order.
type ListAllDiffsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed domain type)
}

// Run executes GET /v1/mods/{id}/diffs and returns all diff entries.
// The diffs are sorted by step_index to ensure correct application order.
func (c ListAllDiffsCommand) Run(ctx context.Context) ([]DiffEntry, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("list all diffs: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("list all diffs: base url required")
	}
	if c.RunID.IsZero() {
		return nil, fmt.Errorf("list all diffs: run id required")
	}

	// Build endpoint: /v1/mods/{id}/diffs
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "mods", url.PathEscape(c.RunID.String()), "diffs")
	if err != nil {
		return nil, fmt.Errorf("list all diffs: build url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("list all diffs: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list all diffs: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "list all diffs")
	}

	// Response structure: {"diffs": [...]}
	var result struct {
		Diffs []DiffEntry `json:"diffs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("list all diffs: decode response: %w", err)
	}

	// Sort diffs by step_index for correct application order.
	// Lower step_index values should be applied first.
	sort.Slice(result.Diffs, func(i, j int) bool {
		return result.Diffs[i].StepIndex < result.Diffs[j].StepIndex
	})

	return result.Diffs, nil
}

// ListRunRepoDiffsCommand fetches repo-scoped diffs via GET /v1/runs/{run_id}/repos/{repo_id}/diffs.
// This is the v1 repo-scoped endpoint that replaces the legacy run-scoped GET /v1/mods/{id}/diffs.
//
// Per roadmap/v1/scope.md:69-71 and roadmap/v1/api.md:263:
// - Returns diffs filtered by repo_id via jobs.repo_id join
// - Diffs for repo A are excluded from repo B listing
// - Response shape is unchanged from legacy endpoint
type ListRunRepoDiffsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed domain type)
	RepoID  string            // Repo ID (NanoID-backed string)
}

// Run executes GET /v1/runs/{run_id}/repos/{repo_id}/diffs and returns all diff entries.
// The diffs are sorted by step_index to ensure correct application order.
func (c ListRunRepoDiffsCommand) Run(ctx context.Context) ([]DiffEntry, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("list run repo diffs: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("list run repo diffs: base url required")
	}
	if c.RunID.IsZero() {
		return nil, fmt.Errorf("list run repo diffs: run id required")
	}
	if strings.TrimSpace(c.RepoID) == "" {
		return nil, fmt.Errorf("list run repo diffs: repo id required")
	}

	// Build endpoint: /v1/runs/{run_id}/repos/{repo_id}/diffs
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "runs", url.PathEscape(c.RunID.String()), "repos", url.PathEscape(c.RepoID), "diffs")
	if err != nil {
		return nil, fmt.Errorf("list run repo diffs: build url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("list run repo diffs: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list run repo diffs: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "list run repo diffs")
	}

	// Response structure: {"diffs": [...]}
	var result struct {
		Diffs []DiffEntry `json:"diffs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("list run repo diffs: decode response: %w", err)
	}

	// Sort diffs by step_index for correct application order.
	// Lower step_index values should be applied first.
	sort.Slice(result.Diffs, func(i, j int) bool {
		return result.Diffs[i].StepIndex < result.Diffs[j].StepIndex
	})

	return result.Diffs, nil
}

// DownloadDiffCommand downloads a single diff patch via GET /v1/diffs/{id}?download=true.
// Returns the decompressed patch bytes ready for application via `git apply`.
type DownloadDiffCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	DiffID  string // Diff ID (UUID string)
}

// Run executes GET /v1/diffs/{id}?download=true and returns the decompressed patch.
func (c DownloadDiffCommand) Run(ctx context.Context) ([]byte, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("download diff: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("download diff: base url required")
	}
	if strings.TrimSpace(c.DiffID) == "" {
		return nil, fmt.Errorf("download diff: diff id required")
	}

	// Build endpoint: /v1/diffs/{id}?download=true
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "diffs", url.PathEscape(c.DiffID))
	if err != nil {
		return nil, fmt.Errorf("download diff: build url: %w", err)
	}
	endpoint += "?download=true"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("download diff: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download diff: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "download diff")
	}

	// Read the gzipped response body.
	gzData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("download diff: read body: %w", err)
	}

	// Decompress the gzipped patch.
	patch, err := decompressGzipBytes(gzData)
	if err != nil {
		return nil, fmt.Errorf("download diff: decompress: %w", err)
	}

	return patch, nil
}

// decompressGzipBytes decompresses gzipped bytes and returns the plaintext content.
func decompressGzipBytes(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = reader.Close() }()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, fmt.Errorf("gzip decompress: %w", err)
	}

	return buf.Bytes(), nil
}
