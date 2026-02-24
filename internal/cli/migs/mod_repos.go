// Package migs provides CLI client implementations for Mods operations.
// This file implements mig repo set management commands (add, list, remove, import).
//
// These commands call the server endpoints:
// - POST /v1/migs/{mod_id}/repos (add repo)
// - GET /v1/migs/{mod_id}/repos (list repos)
// - DELETE /v1/migs/{mod_id}/repos/{repo_id} (delete repo)
// - POST /v1/migs/{mod_id}/repos/bulk (bulk import from CSV)
//
// These commands implement mig repo set management (add, list, remove, import).
package migs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// ModRepoSummary represents a repo in a mig's repo set.
// Matches the server response shape from internal/server/handlers/mods_repos.go.
type ModRepoSummary struct {
	ID        domaintypes.MigRepoID `json:"id"`
	MigID     domaintypes.MigID     `json:"mig_id"`
	RepoURL   string                `json:"repo_url"`
	BaseRef   domaintypes.GitRef    `json:"base_ref"`
	TargetRef domaintypes.GitRef    `json:"target_ref"`
	CreatedAt time.Time             `json:"created_at"`
}

// AddModRepoCommand adds a repo to a mig's repo set.
// Endpoint: POST /v1/migs/{mod_id}/repos
// Adds a repo with URL, base ref, and target ref.
type AddModRepoCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	MigRef    domaintypes.MigRef // Required: mig ID or name.
	RepoURL   string             // Required: git repository URL.
	BaseRef   string             // Required: base git ref.
	TargetRef string             // Required: target git ref.
}

// Run executes POST /v1/migs/{mod_id}/repos to add a repo.
func (c AddModRepoCommand) Run(ctx context.Context) (ModRepoSummary, error) {
	if c.Client == nil {
		return ModRepoSummary{}, fmt.Errorf("mig repo add: http client required")
	}
	if c.BaseURL == nil {
		return ModRepoSummary{}, fmt.Errorf("mig repo add: base url required")
	}
	if err := c.MigRef.Validate(); err != nil {
		return ModRepoSummary{}, fmt.Errorf("mig repo add: mig id is required")
	}
	repoURL := domaintypes.RepoURL(strings.TrimSpace(c.RepoURL))
	if err := repoURL.Validate(); err != nil {
		return ModRepoSummary{}, fmt.Errorf("mig repo add: repo url is required")
	}
	baseRef := domaintypes.GitRef(strings.TrimSpace(c.BaseRef))
	if err := baseRef.Validate(); err != nil {
		return ModRepoSummary{}, fmt.Errorf("mig repo add: base ref is required")
	}
	targetRef := domaintypes.GitRef(strings.TrimSpace(c.TargetRef))
	if err := targetRef.Validate(); err != nil {
		return ModRepoSummary{}, fmt.Errorf("mig repo add: target ref is required")
	}

	// Build request payload with repo_url, base_ref, and target_ref.
	req := struct {
		RepoURL   domaintypes.RepoURL `json:"repo_url"`
		BaseRef   domaintypes.GitRef  `json:"base_ref"`
		TargetRef domaintypes.GitRef  `json:"target_ref"`
	}{
		RepoURL:   repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return ModRepoSummary{}, fmt.Errorf("mig repo add: marshal request: %w", err)
	}

	// POST /v1/migs/{mod_id}/repos
	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), "repos")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return ModRepoSummary{}, fmt.Errorf("mig repo add: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return ModRepoSummary{}, fmt.Errorf("mig repo add: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle 201 Created response.
	if resp.StatusCode == http.StatusCreated {
		var result ModRepoSummary
		if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
			return ModRepoSummary{}, fmt.Errorf("mig repo add: decode response: %w", err)
		}
		return result, nil
	}

	return ModRepoSummary{}, decodeHTTPError(resp, "mig repo add")
}

// ListModReposCommand lists repos in a mig's repo set.
// Endpoint: GET /v1/migs/{mod_id}/repos
// Returns repos with ID, REPO_URL, BASE_REF, TARGET_REF, ADDED_AT.
type ListModReposCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  domaintypes.MigRef // Required: mig ID or name.
}

// Run executes GET /v1/migs/{mod_id}/repos to list repos.
func (c ListModReposCommand) Run(ctx context.Context) ([]ModRepoSummary, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("mig repo list: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("mig repo list: base url required")
	}
	if err := c.MigRef.Validate(); err != nil {
		return nil, fmt.Errorf("mig repo list: mig id is required")
	}

	// GET /v1/migs/{mod_id}/repos
	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), "repos")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("mig repo list: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mig repo list: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "mig repo list")
	}

	// Response structure: {"repos": [...]}
	var result struct {
		Repos []ModRepoSummary `json:"repos"`
	}
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("mig repo list: decode response: %w", err)
	}

	return result.Repos, nil
}

// RemoveModRepoCommand deletes a repo from a mig's repo set.
// Endpoint: DELETE /v1/migs/{mod_id}/repos/{repo_id}
// Refuses deletion if there are historical executions referencing this repo.
type RemoveModRepoCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  domaintypes.MigRef // Required: mig ID or name.
	RepoID  domaintypes.MigRepoID
}

// Run executes DELETE /v1/migs/{mod_id}/repos/{repo_id} to delete a repo.
func (c RemoveModRepoCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return fmt.Errorf("mig repo remove: http client required")
	}
	if c.BaseURL == nil {
		return fmt.Errorf("mig repo remove: base url required")
	}
	if err := c.MigRef.Validate(); err != nil {
		return fmt.Errorf("mig repo remove: mig id is required")
	}
	if c.RepoID.IsZero() {
		return fmt.Errorf("mig repo remove: repo id is required")
	}

	// DELETE /v1/migs/{mod_id}/repos/{repo_id}
	endpoint := c.BaseURL.JoinPath(
		"v1",
		"migs",
		c.MigRef.String(),
		"repos",
		c.RepoID.String(),
	)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("mig repo remove: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mig repo remove: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 204 No Content indicates success.
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return decodeHTTPError(resp, "mig repo remove")
}

// ImportModReposCommand bulk imports repos for a mig from CSV.
// Endpoint: POST /v1/migs/{mod_id}/repos/bulk
// Imports repos from CSV with header: repo_url,base_ref,target_ref.
type ImportModReposCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  domaintypes.MigRef // Required: mig ID or name.
	CSVData []byte             // Required: CSV content with header: repo_url,base_ref,target_ref
}

// ImportModReposResult contains the response from bulk importing repos.
type ImportModReposResult struct {
	Created int           `json:"created"`
	Updated int           `json:"updated"`
	Failed  int           `json:"failed"`
	Errors  []ImportError `json:"errors"`
}

// ImportError represents a per-line error from CSV import.
type ImportError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// Run executes POST /v1/migs/{mod_id}/repos/bulk to import repos from CSV.
func (c ImportModReposCommand) Run(ctx context.Context) (ImportModReposResult, error) {
	if c.Client == nil {
		return ImportModReposResult{}, fmt.Errorf("mig repo import: http client required")
	}
	if c.BaseURL == nil {
		return ImportModReposResult{}, fmt.Errorf("mig repo import: base url required")
	}
	if err := c.MigRef.Validate(); err != nil {
		return ImportModReposResult{}, fmt.Errorf("mig repo import: mig id is required")
	}
	if len(c.CSVData) == 0 {
		return ImportModReposResult{}, fmt.Errorf("mig repo import: csv data is required")
	}

	// POST /v1/migs/{mod_id}/repos/bulk with Content-Type: text/csv
	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), "repos", "bulk")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(c.CSVData))
	if err != nil {
		return ImportModReposResult{}, fmt.Errorf("mig repo import: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "text/csv")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return ImportModReposResult{}, fmt.Errorf("mig repo import: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle 200 OK response (bulk import always returns 200 with counts).
	if resp.StatusCode == http.StatusOK {
		var result ImportModReposResult
		if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
			return ImportModReposResult{}, fmt.Errorf("mig repo import: decode response: %w", err)
		}
		return result, nil
	}

	return ImportModReposResult{}, decodeHTTPError(resp, "mig repo import")
}
