// Package migs provides CLI client implementations for Migs operations.
// This file implements mig repo set management commands (add, list, remove, import).
//
// These commands call the server endpoints:
// - POST /v1/migs/{mig_id}/repos (add repo)
// - GET /v1/migs/{mig_id}/repos (list repos)
// - DELETE /v1/migs/{mig_id}/repos/{repo_id} (delete repo)
// - POST /v1/migs/{mig_id}/repos/bulk (bulk import from CSV)
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

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// AddMigRepoCommand adds a repo to a mig's repo set.
// Endpoint: POST /v1/migs/{mig_id}/repos
// Adds a repo with URL, base ref, and target ref.
type AddMigRepoCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	MigRef    domaintypes.MigRef // Required: mig ID or name.
	RepoURL   string             // Required: git repository URL.
	BaseRef   string             // Required: base git ref.
	TargetRef string             // Required: target git ref.
}

// Run executes POST /v1/migs/{mig_id}/repos to add a repo.
func (c AddMigRepoCommand) Run(ctx context.Context) (domainapi.MigRepoSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domainapi.MigRepoSummary{}, fmt.Errorf("mig repo add: %w", err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return domainapi.MigRepoSummary{}, fmt.Errorf("mig repo add: mig id is required")
	}
	repoURL := domaintypes.RepoURL(strings.TrimSpace(c.RepoURL))
	if err := repoURL.Validate(); err != nil {
		return domainapi.MigRepoSummary{}, fmt.Errorf("mig repo add: repo url is required")
	}
	baseRef := domaintypes.GitRef(strings.TrimSpace(c.BaseRef))
	if err := baseRef.Validate(); err != nil {
		return domainapi.MigRepoSummary{}, fmt.Errorf("mig repo add: base ref is required")
	}
	targetRef := domaintypes.GitRef(strings.TrimSpace(c.TargetRef))
	if err := targetRef.Validate(); err != nil {
		return domainapi.MigRepoSummary{}, fmt.Errorf("mig repo add: target ref is required")
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
		return domainapi.MigRepoSummary{}, fmt.Errorf("mig repo add: marshal request: %w", err)
	}

	// POST /v1/migs/{mig_id}/repos
	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), "repos")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return domainapi.MigRepoSummary{}, fmt.Errorf("mig repo add: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return domainapi.MigRepoSummary{}, fmt.Errorf("mig repo add: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	// Handle 201 Created response.
	if resp.StatusCode == http.StatusCreated {
		var result domainapi.MigRepoSummary
		if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
			return domainapi.MigRepoSummary{}, fmt.Errorf("mig repo add: decode response: %w", err)
		}
		return result, nil
	}

	return domainapi.MigRepoSummary{}, httpx.WrapError("mig repo add", resp.Status, resp.Body)
}

// ListMigReposCommand lists repos in a mig's repo set.
// Endpoint: GET /v1/migs/{mig_id}/repos
// Returns repos with ID, REPO_URL, BASE_REF, TARGET_REF, ADDED_AT.
type ListMigReposCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  domaintypes.MigRef // Required: mig ID or name.
}

// Run executes GET /v1/migs/{mig_id}/repos to list repos.
func (c ListMigReposCommand) Run(ctx context.Context) ([]domainapi.MigRepoSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("mig repo list: %w", err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return nil, fmt.Errorf("mig repo list: mig id is required")
	}

	// GET /v1/migs/{mig_id}/repos
	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), "repos")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("mig repo list: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mig repo list: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("mig repo list", resp.Status, resp.Body)
	}

	var result domainapi.MigRepoListResponse
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("mig repo list: decode response: %w", err)
	}

	return result.Repos, nil
}

// RemoveMigRepoCommand deletes a repo from a mig's repo set.
// Endpoint: DELETE /v1/migs/{mig_id}/repos/{repo_id}
// Refuses deletion if there are historical executions referencing this repo.
type RemoveMigRepoCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  domaintypes.MigRef // Required: mig ID or name.
	RepoID  domaintypes.MigRepoID
}

// Run executes DELETE /v1/migs/{mig_id}/repos/{repo_id} to delete a repo.
func (c RemoveMigRepoCommand) Run(ctx context.Context) error {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return fmt.Errorf("mig repo remove: %w", err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return fmt.Errorf("mig repo remove: mig id is required")
	}
	if c.RepoID.IsZero() {
		return fmt.Errorf("mig repo remove: repo id is required")
	}

	// DELETE /v1/migs/{mig_id}/repos/{repo_id}
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
	defer httpx.DrainAndClose(resp)

	// 204 No Content indicates success.
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return httpx.WrapError("mig repo remove", resp.Status, resp.Body)
}

// ImportMigReposCommand bulk imports repos for a mig from CSV.
// Endpoint: POST /v1/migs/{mig_id}/repos/bulk
// Imports repos from CSV with header: repo_url,base_ref,target_ref.
type ImportMigReposCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  domaintypes.MigRef // Required: mig ID or name.
	CSVData []byte             // Required: CSV content with header: repo_url,base_ref,target_ref
}

// ImportMigReposResult contains the response from bulk importing repos.
type ImportMigReposResult struct {
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

// Run executes POST /v1/migs/{mig_id}/repos/bulk to import repos from CSV.
func (c ImportMigReposCommand) Run(ctx context.Context) (ImportMigReposResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return ImportMigReposResult{}, fmt.Errorf("mig repo import: %w", err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return ImportMigReposResult{}, fmt.Errorf("mig repo import: mig id is required")
	}
	if len(c.CSVData) == 0 {
		return ImportMigReposResult{}, fmt.Errorf("mig repo import: csv data is required")
	}

	// POST /v1/migs/{mig_id}/repos/bulk with Content-Type: text/csv
	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), "repos", "bulk")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(c.CSVData))
	if err != nil {
		return ImportMigReposResult{}, fmt.Errorf("mig repo import: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "text/csv")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return ImportMigReposResult{}, fmt.Errorf("mig repo import: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	// Handle 200 OK response (bulk import always returns 200 with counts).
	if resp.StatusCode == http.StatusOK {
		var result ImportMigReposResult
		if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
			return ImportMigReposResult{}, fmt.Errorf("mig repo import: decode response: %w", err)
		}
		return result, nil
	}

	return ImportMigReposResult{}, httpx.WrapError("mig repo import", resp.Status, resp.Body)
}
