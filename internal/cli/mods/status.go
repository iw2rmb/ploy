// status.go provides CLI client implementations for fetching diffs and patches.
//
// This file implements helpers used by `ploy run pull` and `ploy mod pull`:
//   - ListRunRepoDiffsCommand: Fetches repo-scoped diffs via GET /v1/runs/{run_id}/repos/{repo_id}/diffs (v1).
//   - DownloadDiffCommand: Downloads a single diff via GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>.
package mods

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// DiffEntry represents a single diff record from the list diffs response.
type DiffEntry struct {
	ID        string                  `json:"id"`
	JobID     domaintypes.JobID       `json:"job_id"`
	CreatedAt string                  `json:"created_at"`
	Size      int                     `json:"gzipped_size"`
	Summary   domaintypes.DiffSummary `json:"summary,omitempty"`
}

// ListRunRepoDiffsCommand fetches repo-scoped diffs via GET /v1/runs/{run_id}/repos/{repo_id}/diffs.
// This is the v1 repo-scoped diffs listing endpoint.
//
// Returns diffs filtered by repo_id via jobs.repo_id join.
// Diffs for repo A are excluded from repo B listing.
// Response shape is unchanged from legacy endpoint.
type ListRunRepoDiffsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed domain type)
	RepoID  string            // Repo ID (NanoID-backed string)
}

// Run executes GET /v1/runs/{run_id}/repos/{repo_id}/diffs and returns all diff entries.
// Diffs are returned in server-provided order (ordered by step_index, then created_at).
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
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("list run repo diffs: decode response: %w", err)
	}

	return result.Diffs, nil
}

// DownloadDiffCommand downloads a single diff patch via:
// GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>
// Returns the decompressed patch bytes ready for application via `git apply`.
type DownloadDiffCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed domain type)
	RepoID  string            // Repo ID (NanoID-backed string)
	DiffID  string            // Diff ID (UUID string)
}

// Run executes GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>
// and returns the decompressed patch.
func (c DownloadDiffCommand) Run(ctx context.Context) ([]byte, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("download diff: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("download diff: base url required")
	}
	if c.RunID.IsZero() {
		return nil, fmt.Errorf("download diff: run id required")
	}
	if strings.TrimSpace(c.RepoID) == "" {
		return nil, fmt.Errorf("download diff: repo id required")
	}
	if strings.TrimSpace(c.DiffID) == "" {
		return nil, fmt.Errorf("download diff: diff id required")
	}

	// Build endpoint: /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "runs", url.PathEscape(c.RunID.String()), "repos", url.PathEscape(c.RepoID), "diffs")
	if err != nil {
		return nil, fmt.Errorf("download diff: build url: %w", err)
	}
	q := url.Values{}
	q.Set("download", "true")
	q.Set("diff_id", strings.TrimSpace(c.DiffID))
	endpoint += "?" + q.Encode()

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
	gzData, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxDownloadBodyBytes))
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
