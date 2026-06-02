// Package migs provides CLI client implementations for Migs operations.
// This file implements the mig run command for creating waves from a mig project.
//
// This command calls POST /v1/migs/{mig_id}/waves with repo selection.
// Implements the control-plane call for `ploy mig run` after the public CLI has
// resolved any positional repo selectors to canonical repo URLs.
// User-facing selector resolution happens in internal/cli/mig before this
// command sends canonical repo URLs to the control-plane API.
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

// CreateMigRunCommand creates a launch wave from a mig project with repo selection.
// Endpoint: POST /v1/migs/{mig_id}/waves
// Creates a wave with run selection based on mode: all, explicit, or failed.
type CreateMigRunCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	MigRef    domaintypes.MigRef // Required: mig ID or name.
	RepoURLs  []string           // Optional: canonical repo URLs for "explicit" mode.
	Failed    bool               // If true, use "failed" mode; otherwise "all" or "explicit".
	CreatedBy *string            // Optional: creator identifier.
}

// CreateMigRunResult contains the response from creating a mig wave.
type CreateMigRunResult struct {
	WaveID   domaintypes.WaveID `json:"wave_id"`
	MigID    domaintypes.MigID  `json:"mig_id"`
	SpecID   domaintypes.SpecID `json:"spec_id"`
	RunCount int                `json:"run_count"`
}

// Run executes POST /v1/migs/{mig_id}/waves to create a wave with repo selection.
// Selection behavior:
//   - canonical repo URLs select explicit repos by repo_url identity within the mig
//   - --failed selects repos with last terminal state Fail
//   - omitted repo selection selects all repos in the mig repo set
func (c CreateMigRunCommand) Run(ctx context.Context) (CreateMigRunResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return CreateMigRunResult{}, fmt.Errorf("mig run: %w", err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return CreateMigRunResult{}, fmt.Errorf("mig run: mig id is required")
	}

	if c.Failed && len(c.RepoURLs) > 0 {
		return CreateMigRunResult{}, fmt.Errorf("mig run: failed and explicit repos are mutually exclusive")
	}

	// Determine repo_selector mode based on selection input.
	var mode string
	var repoURLs []domaintypes.RepoURL
	switch {
	case c.Failed:
		mode = "failed"
	case len(c.RepoURLs) > 0:
		mode = "explicit"
		for _, raw := range c.RepoURLs {
			u := domaintypes.RepoURL(strings.TrimSpace(raw))
			if err := u.Validate(); err != nil {
				return CreateMigRunResult{}, fmt.Errorf("mig run: explicit repo must be a valid repo url")
			}
			repoURLs = append(repoURLs, u)
		}
	default:
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

	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), "waves")
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
