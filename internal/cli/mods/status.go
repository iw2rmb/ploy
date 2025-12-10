// status.go provides CLI client implementations for fetching run details and diffs.
//
// This file implements helpers used by `ploy mod run pull`:
//   - FetchRunWithCommitSHA: Fetches full run details (including commit_sha)
//     via GET /v1/runs/{id}.
//   - ListAllDiffsCommand: Fetches all diffs for a run via GET /v1/mods/{id}/diffs.
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

// FetchRunWithCommitSHA fetches full run details including commit_sha via GET /v1/runs/{id}.
// This is used when we need the commit_sha that isn't exposed in the mods API.
type FetchRunWithCommitSHA struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   string // Run ID (KSUID-backed string)
}

// RunDetails contains the full run record with commit_sha.
type RunDetails struct {
	ID        string  `json:"id"`
	RepoURL   string  `json:"repo_url"`
	Status    string  `json:"status"`
	BaseRef   string  `json:"base_ref"`
	TargetRef string  `json:"target_ref"`
	CommitSHA *string `json:"commit_sha,omitempty"`
}

// Run executes GET /v1/runs/{id} and returns the run details including commit_sha.
func (c FetchRunWithCommitSHA) Run(ctx context.Context) (*RunDetails, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("fetch run: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("fetch run: base url required")
	}
	if strings.TrimSpace(c.RunID) == "" {
		return nil, fmt.Errorf("fetch run: run id required")
	}

	// Build endpoint: /v1/runs/{id}
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "runs", url.PathEscape(c.RunID))
	if err != nil {
		return nil, fmt.Errorf("fetch run: build url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch run: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch run: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "fetch run")
	}

	var result RunDetails
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("fetch run: decode response: %w", err)
	}

	return &result, nil
}

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
	RunID   string // Run ID (KSUID-backed string, execution run id)
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
	if strings.TrimSpace(c.RunID) == "" {
		return nil, fmt.Errorf("list all diffs: run id required")
	}

	// Build endpoint: /v1/mods/{id}/diffs
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "mods", url.PathEscape(c.RunID), "diffs")
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
