// Package migs provides CLI client implementations for Migs operations.
// This file implements the mig run command for creating runs from a mig project.
//
// This command calls POST /v1/migs/{mod_id}/runs with repo selection.
// Implements: ploy mig run <mig-id|name> [--repo <repo-url> ...] [--failed]
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
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// CreateMigRunCommand creates a batch run from a mig project with repo selection.
// Endpoint: POST /v1/migs/{mod_id}/runs
// Creates a run with repo selection based on mode: all, explicit, or failed.
type CreateMigRunCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	MigRef    domaintypes.MigRef // Required: mig ID or name.
	RepoURLs  []string           // Optional: explicit repo URLs for "explicit" mode.
	Failed    bool               // If true, use "failed" mode; otherwise "all" or "explicit".
	CreatedBy *string            // Optional: creator identifier.
}

// CreateMigRunResult contains the response from creating a mig run.
type CreateMigRunResult struct {
	RunID domaintypes.RunID `json:"run_id"`
}

// Run executes POST /v1/migs/{mod_id}/runs to create a run with repo selection.
// Flag behavior:
//   - --repo ... selects explicit repos (by repo_url identity within the mig)
//   - --failed selects repos with last terminal state Fail
//   - omitted selects all repos in the mig repo set
func (c CreateMigRunCommand) Run(ctx context.Context) (CreateMigRunResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return CreateMigRunResult{}, fmt.Errorf("mig run: %w", err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return CreateMigRunResult{}, fmt.Errorf("mig run: mig id is required")
	}

	// Validate flag mutual exclusion: --failed and --repo cannot both be specified.
	if c.Failed && len(c.RepoURLs) > 0 {
		return CreateMigRunResult{}, fmt.Errorf("mig run: --failed and --repo are mutually exclusive")
	}

	// Determine repo_selector mode based on flags.
	var mode string
	var repoURLs []domaintypes.RepoURL
	switch {
	case c.Failed:
		// --failed → repos whose last terminal state is Fail.
		mode = "failed"
	case len(c.RepoURLs) > 0:
		// --repo ... → explicit repos by URL.
		mode = "explicit"
		for _, raw := range c.RepoURLs {
			u := domaintypes.RepoURL(strings.TrimSpace(raw))
			if err := u.Validate(); err != nil {
				return CreateMigRunResult{}, fmt.Errorf("mig run: --repo must be a valid repo url")
			}
			repoURLs = append(repoURLs, u)
		}
	default:
		// No flags → all repos in the mig repo set.
		mode = "all"
	}

	// Build request payload with repo_selector mode and optional repos list.
	req := struct {
		RepoSelector struct {
			Mode  string                `json:"mode"`
			Repos []domaintypes.RepoURL `json:"repos,omitempty"`
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
		return CreateMigRunResult{}, fmt.Errorf("mig run: marshal request: %w", err)
	}

	// POST /v1/migs/{mod_id}/runs
	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), "runs")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return CreateMigRunResult{}, fmt.Errorf("mig run: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return CreateMigRunResult{}, fmt.Errorf("mig run: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	// Handle 201 Created response.
	if resp.StatusCode == http.StatusCreated {
		var result CreateMigRunResult
		if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
			return CreateMigRunResult{}, fmt.Errorf("mig run: decode response: %w", err)
		}
		return result, nil
	}

	return CreateMigRunResult{}, httpx.WrapError("mig run", resp.Status, resp.Body)
}
