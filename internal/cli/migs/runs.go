// Package migs provides CLI client implementations for Migs operations.
// This file implements the run lifecycle client for create/list/stop/status
// operations against the control-plane /v1/runs endpoints.
//
// RunClient encapsulates HTTP calls to the control-plane and maps responses
// to domain types for CLI consumption. It follows the same Command pattern
// used by SubmitCommand and InspectCommand.
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
	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// CreateRunCommand creates a new run with a supplied spec.
// The run creates a single repository execution.
type CreateRunCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	Name      *string         // Optional run name.
	Spec      json.RawMessage // Migs spec (YAML/JSON parsed to JSON).
	CreatedBy *string         // Optional creator identifier.
	RepoURL   string          // Initial repo URL (required by server).
	BaseRef   string          // Initial base ref.
}

// Run executes POST /v1/runs to submit the initial repo for a run.
// Returns the created run summary on success.
func (c CreateRunCommand) Run(ctx context.Context) (domaintypes.RunSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("run create: %w", err)
	}

	repoURL := strings.TrimSpace(c.RepoURL)
	if err := domaintypes.RepoURL(repoURL).Validate(); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("run create: repo_url: %w", err)
	}
	baseRef := strings.TrimSpace(c.BaseRef)
	if err := domaintypes.GitRef(baseRef).Validate(); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("run create: base_ref: %w", err)
	}
	if len(c.Spec) == 0 {
		return domaintypes.RunSummary{}, fmt.Errorf("run create: spec is required")
	}

	// Build the request payload matching server's expected format.
	// Server uses POST /v1/runs for single-repo run submission.
	req := struct {
		RepoURL   string          `json:"repo_url"`
		Ref       string          `json:"ref"`
		Spec      json.RawMessage `json:"spec"`
		CreatedBy *string         `json:"created_by,omitempty"`
	}{
		RepoURL:   repoURL,
		Ref:       baseRef,
		Spec:      c.Spec,
		CreatedBy: c.CreatedBy,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("run create: marshal request: %w", err)
	}

	// POST /v1/runs to create the run.
	endpoint := c.BaseURL.JoinPath("v1", "runs")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("run create: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("run create: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	// Handle 201 Created response from server.
	if resp.StatusCode == http.StatusCreated {
		var srvResp struct {
			RunID domaintypes.RunID `json:"run_id"`
		}
		if err := httpx.DecodeResponseJSON(resp.Body, &srvResp, httpx.MaxJSONBodyBytes); err != nil {
			return domaintypes.RunSummary{}, fmt.Errorf("run create: decode response: %w", err)
		}

		summary, err := runs.GetStatusCommand{
			Client:  c.Client,
			BaseURL: c.BaseURL,
			RunID:   srvResp.RunID,
		}.Run(ctx)
		if err != nil {
			return domaintypes.RunSummary{}, fmt.Errorf("run create: fetch run summary: %w", err)
		}

		return summary, nil
	}

	// Non-success: read error body and return error.
	return domaintypes.RunSummary{}, httpx.WrapError("run create", resp.Status, resp.Body)
}

// ListRunsCommand lists runs from the control plane.
type ListRunsCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	Limit     int32 // Max results to return (default 50, max 100).
	Offset    int32 // Number of results to skip.
	RepoURL   string
	CreatedBy string
	All       bool
}

// Run executes GET /v1/runs to list runs with pagination.
func (c ListRunsCommand) Run(ctx context.Context) ([]domaintypes.RunSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("run list: %w", err)
	}

	// Build endpoint with query params.
	endpoint := c.BaseURL.JoinPath("v1", "runs")
	q := endpoint.Query()
	if c.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", c.Limit))
	}
	if c.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", c.Offset))
	}
	if repoURL := strings.TrimSpace(c.RepoURL); repoURL != "" {
		q.Set("repo_url", repoURL)
	}
	if createdBy := strings.TrimSpace(c.CreatedBy); createdBy != "" {
		q.Set("created_by", createdBy)
	}
	if c.All {
		q.Set("all", "true")
	}
	endpoint.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("run list: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("run list: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("run list", resp.Status, resp.Body)
	}

	// Response structure: {"runs": [...]}
	var result struct {
		Runs []domaintypes.RunSummary `json:"runs"`
	}
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run list: decode response: %w", err)
	}

	return result.Runs, nil
}
