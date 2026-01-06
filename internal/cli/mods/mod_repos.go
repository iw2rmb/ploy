// Package mods provides CLI client implementations for Mods operations.
// This file implements mod repo set management commands (add, list, remove, import).
//
// These commands call the server endpoints:
// - POST /v1/mods/{mod_id}/repos (add repo)
// - GET /v1/mods/{mod_id}/repos (list repos)
// - DELETE /v1/mods/{mod_id}/repos/{repo_id} (delete repo)
// - POST /v1/mods/{mod_id}/repos/bulk (bulk import from CSV)
//
// Per roadmap/v1/cli.md:62-99, these commands implement mod repo set management.
package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ModRepoSummary represents a repo in a mod's repo set.
// Matches the server response shape from internal/server/handlers/mods_repos.go.
type ModRepoSummary struct {
	ID        string `json:"id"`
	ModID     string `json:"mod_id"`
	RepoURL   string `json:"repo_url"`
	BaseRef   string `json:"base_ref"`
	TargetRef string `json:"target_ref"`
	CreatedAt string `json:"created_at"`
}

// AddModRepoCommand adds a repo to a mod's repo set.
// Endpoint: POST /v1/mods/{mod_id}/repos
// Per roadmap/v1/cli.md:72-75, this adds a repo with URL and refs.
type AddModRepoCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	ModID     string // Required: mod ID or name.
	RepoURL   string // Required: git repository URL.
	BaseRef   string // Required: base git ref.
	TargetRef string // Required: target git ref.
}

// Run executes POST /v1/mods/{mod_id}/repos to add a repo.
func (c AddModRepoCommand) Run(ctx context.Context) (ModRepoSummary, error) {
	if c.Client == nil {
		return ModRepoSummary{}, fmt.Errorf("mod repo add: http client required")
	}
	if c.BaseURL == nil {
		return ModRepoSummary{}, fmt.Errorf("mod repo add: base url required")
	}
	if strings.TrimSpace(c.ModID) == "" {
		return ModRepoSummary{}, fmt.Errorf("mod repo add: mod id is required")
	}
	if strings.TrimSpace(c.RepoURL) == "" {
		return ModRepoSummary{}, fmt.Errorf("mod repo add: repo url is required")
	}
	if strings.TrimSpace(c.BaseRef) == "" {
		return ModRepoSummary{}, fmt.Errorf("mod repo add: base ref is required")
	}
	if strings.TrimSpace(c.TargetRef) == "" {
		return ModRepoSummary{}, fmt.Errorf("mod repo add: target ref is required")
	}

	// Build request payload per roadmap/v1/api.md:158-162.
	req := struct {
		RepoURL   string `json:"repo_url"`
		BaseRef   string `json:"base_ref"`
		TargetRef string `json:"target_ref"`
	}{
		RepoURL:   strings.TrimSpace(c.RepoURL),
		BaseRef:   strings.TrimSpace(c.BaseRef),
		TargetRef: strings.TrimSpace(c.TargetRef),
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return ModRepoSummary{}, fmt.Errorf("mod repo add: marshal request: %w", err)
	}

	// POST /v1/mods/{mod_id}/repos
	endpoint := c.BaseURL.JoinPath("/v1/mods", url.PathEscape(strings.TrimSpace(c.ModID)), "repos")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return ModRepoSummary{}, fmt.Errorf("mod repo add: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return ModRepoSummary{}, fmt.Errorf("mod repo add: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle 201 Created response.
	if resp.StatusCode == http.StatusCreated {
		var result ModRepoSummary
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return ModRepoSummary{}, fmt.Errorf("mod repo add: decode response: %w", err)
		}
		return result, nil
	}

	return ModRepoSummary{}, decodeHTTPError(resp, "mod repo add")
}

// ListModReposCommand lists repos in a mod's repo set.
// Endpoint: GET /v1/mods/{mod_id}/repos
// Per roadmap/v1/cli.md:77-79, this lists repos with ID, REPO_URL, BASE_REF, TARGET_REF, ADDED_AT.
type ListModReposCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	ModID   string // Required: mod ID or name.
}

// Run executes GET /v1/mods/{mod_id}/repos to list repos.
func (c ListModReposCommand) Run(ctx context.Context) ([]ModRepoSummary, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("mod repo list: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("mod repo list: base url required")
	}
	if strings.TrimSpace(c.ModID) == "" {
		return nil, fmt.Errorf("mod repo list: mod id is required")
	}

	// GET /v1/mods/{mod_id}/repos
	endpoint := c.BaseURL.JoinPath("/v1/mods", url.PathEscape(strings.TrimSpace(c.ModID)), "repos")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("mod repo list: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mod repo list: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "mod repo list")
	}

	// Response structure: {"repos": [...]}
	var result struct {
		Repos []ModRepoSummary `json:"repos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("mod repo list: decode response: %w", err)
	}

	return result.Repos, nil
}

// RemoveModRepoCommand deletes a repo from a mod's repo set.
// Endpoint: DELETE /v1/mods/{mod_id}/repos/{repo_id}
// Per roadmap/v1/cli.md:81-84, this refuses deletion if there are historical executions.
type RemoveModRepoCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	ModID   string // Required: mod ID or name.
	RepoID  string // Required: repo ID to delete.
}

// Run executes DELETE /v1/mods/{mod_id}/repos/{repo_id} to delete a repo.
func (c RemoveModRepoCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return fmt.Errorf("mod repo remove: http client required")
	}
	if c.BaseURL == nil {
		return fmt.Errorf("mod repo remove: base url required")
	}
	if strings.TrimSpace(c.ModID) == "" {
		return fmt.Errorf("mod repo remove: mod id is required")
	}
	if strings.TrimSpace(c.RepoID) == "" {
		return fmt.Errorf("mod repo remove: repo id is required")
	}

	// DELETE /v1/mods/{mod_id}/repos/{repo_id}
	endpoint := c.BaseURL.JoinPath(
		"/v1/mods",
		url.PathEscape(strings.TrimSpace(c.ModID)),
		"repos",
		url.PathEscape(strings.TrimSpace(c.RepoID)),
	)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("mod repo remove: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mod repo remove: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 204 No Content indicates success.
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return decodeHTTPError(resp, "mod repo remove")
}

// ImportModReposCommand bulk imports repos for a mod from CSV.
// Endpoint: POST /v1/mods/{mod_id}/repos/bulk
// Per roadmap/v1/cli.md:86-98, this imports repos from CSV with specific parsing rules.
type ImportModReposCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	ModID   string // Required: mod ID or name.
	CSVData []byte // Required: CSV content with header: repo_url,base_ref,target_ref
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

// Run executes POST /v1/mods/{mod_id}/repos/bulk to import repos from CSV.
func (c ImportModReposCommand) Run(ctx context.Context) (ImportModReposResult, error) {
	if c.Client == nil {
		return ImportModReposResult{}, fmt.Errorf("mod repo import: http client required")
	}
	if c.BaseURL == nil {
		return ImportModReposResult{}, fmt.Errorf("mod repo import: base url required")
	}
	if strings.TrimSpace(c.ModID) == "" {
		return ImportModReposResult{}, fmt.Errorf("mod repo import: mod id is required")
	}
	if len(c.CSVData) == 0 {
		return ImportModReposResult{}, fmt.Errorf("mod repo import: csv data is required")
	}

	// POST /v1/mods/{mod_id}/repos/bulk with Content-Type: text/csv
	endpoint := c.BaseURL.JoinPath("/v1/mods", url.PathEscape(strings.TrimSpace(c.ModID)), "repos", "bulk")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(c.CSVData))
	if err != nil {
		return ImportModReposResult{}, fmt.Errorf("mod repo import: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "text/csv")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return ImportModReposResult{}, fmt.Errorf("mod repo import: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle 200 OK response (bulk import always returns 200 with counts).
	if resp.StatusCode == http.StatusOK {
		var result ImportModReposResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return ImportModReposResult{}, fmt.Errorf("mod repo import: decode response: %w", err)
		}
		return result, nil
	}

	return ImportModReposResult{}, decodeHTTPError(resp, "mod repo import")
}
