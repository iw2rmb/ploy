// Package mods provides CLI client implementations for Mods operations.
// This file implements the mod run command for creating runs from a mod project.
//
// This command calls POST /v1/mods/{mod_id}/runs with repo selection.
//
// Per roadmap/v1/cli.md:102-119, this command implements:
// - ploy mod run <mod-id|name> [--repo <repo-url> ...] [--failed]
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

// CreateModRunCommand creates a batch run from a mod project with repo selection.
// Endpoint: POST /v1/mods/{mod_id}/runs
// Per roadmap/v1/api.md:202-223, this creates a run with repo selection.
type CreateModRunCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	ModID     string   // Required: mod ID or name.
	RepoURLs  []string // Optional: explicit repo URLs for "explicit" mode.
	Failed    bool     // If true, use "failed" mode; otherwise "all" or "explicit".
	CreatedBy *string  // Optional: creator identifier.
}

// CreateModRunResult contains the response from creating a mod run.
type CreateModRunResult struct {
	RunID string `json:"run_id"`
}

// Run executes POST /v1/mods/{mod_id}/runs to create a run with repo selection.
// Per roadmap/v1/cli.md:112-116:
// - --repo ... → explicit repos (by repo_url identity within the mod)
// - --failed → repos with last terminal state Fail
// - omitted → all repos in the mod repo set
func (c CreateModRunCommand) Run(ctx context.Context) (CreateModRunResult, error) {
	if c.Client == nil {
		return CreateModRunResult{}, fmt.Errorf("mod run: http client required")
	}
	if c.BaseURL == nil {
		return CreateModRunResult{}, fmt.Errorf("mod run: base url required")
	}
	if strings.TrimSpace(c.ModID) == "" {
		return CreateModRunResult{}, fmt.Errorf("mod run: mod id is required")
	}

	// Validate flag mutual exclusion: --failed and --repo cannot both be specified.
	if c.Failed && len(c.RepoURLs) > 0 {
		return CreateModRunResult{}, fmt.Errorf("mod run: --failed and --repo are mutually exclusive")
	}

	// Determine repo_selector mode based on flags.
	var mode string
	var repoURLs []string
	switch {
	case c.Failed:
		// --failed → repos whose last terminal state is Fail.
		mode = "failed"
	case len(c.RepoURLs) > 0:
		// --repo ... → explicit repos by URL.
		mode = "explicit"
		repoURLs = c.RepoURLs
	default:
		// No flags → all repos in the mod repo set.
		mode = "all"
	}

	// Build request payload per roadmap/v1/api.md:209-214.
	req := struct {
		RepoSelector struct {
			Mode  string   `json:"mode"`
			Repos []string `json:"repos,omitempty"`
		} `json:"repo_selector"`
		CreatedBy *string `json:"created_by,omitempty"`
	}{
		CreatedBy: c.CreatedBy,
	}
	req.RepoSelector.Mode = mode
	if mode == "explicit" {
		req.RepoSelector.Repos = repoURLs
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return CreateModRunResult{}, fmt.Errorf("mod run: marshal request: %w", err)
	}

	// POST /v1/mods/{mod_id}/runs
	endpoint := c.BaseURL.JoinPath("/v1/mods", url.PathEscape(strings.TrimSpace(c.ModID)), "runs")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return CreateModRunResult{}, fmt.Errorf("mod run: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return CreateModRunResult{}, fmt.Errorf("mod run: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle 201 Created response.
	if resp.StatusCode == http.StatusCreated {
		var result CreateModRunResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return CreateModRunResult{}, fmt.Errorf("mod run: decode response: %w", err)
		}
		return result, nil
	}

	return CreateModRunResult{}, decodeHTTPError(resp, "mod run")
}
