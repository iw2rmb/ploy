// Package migs provides CLI client implementations for Mods operations.
// This file implements the batch run lifecycle client for create/list/stop/status
// operations against the control-plane /v1/runs endpoints.
//
// BatchClient encapsulates HTTP calls to the control-plane and maps responses
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

// CreateBatchCommand creates a new batch run with a shared spec.
// The batch run serves as the parent for multiple repo executions.
type CreateBatchCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	Name      *string         // Optional batch name.
	Spec      json.RawMessage // Mods spec (YAML/JSON parsed to JSON).
	CreatedBy *string         // Optional creator identifier.
	RepoURL   string          // Initial repo URL for batch (required by server).
	BaseRef   string          // Initial base ref.
	TargetRef string          // Initial target ref.
}

// Run executes POST /v1/runs to submit the initial repo for a run.
// Returns the created batch summary on success.
func (c CreateBatchCommand) Run(ctx context.Context) (domaintypes.RunSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("batch create: %w", err)
	}

	repoURL := strings.TrimSpace(c.RepoURL)
	if err := domaintypes.RepoURL(repoURL).Validate(); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("batch create: repo_url: %w", err)
	}
	baseRef := strings.TrimSpace(c.BaseRef)
	if err := domaintypes.GitRef(baseRef).Validate(); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("batch create: base_ref: %w", err)
	}
	targetRef := strings.TrimSpace(c.TargetRef)
	if err := domaintypes.GitRef(targetRef).Validate(); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("batch create: target_ref: %w", err)
	}
	if len(c.Spec) == 0 {
		return domaintypes.RunSummary{}, fmt.Errorf("batch create: spec is required")
	}

	// Build the request payload matching server's expected format.
	// Server uses POST /v1/runs for single-repo run submission.
	req := struct {
		RepoURL   string          `json:"repo_url"`
		BaseRef   string          `json:"base_ref"`
		TargetRef string          `json:"target_ref"`
		Spec      json.RawMessage `json:"spec"`
		CreatedBy *string         `json:"created_by,omitempty"`
	}{
		RepoURL:   repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
		Spec:      c.Spec,
		CreatedBy: c.CreatedBy,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("batch create: marshal request: %w", err)
	}

	// POST /v1/runs to create the run.
	endpoint := c.BaseURL.JoinPath("v1", "runs")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("batch create: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("batch create: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	// Handle 201 Created response from server.
	if resp.StatusCode == http.StatusCreated {
		var srvResp struct {
			RunID domaintypes.RunID `json:"run_id"`
		}
		if err := httpx.DecodeJSON(resp.Body, &srvResp, httpx.MaxJSONBodyBytes); err != nil {
			return domaintypes.RunSummary{}, fmt.Errorf("batch create: decode response: %w", err)
		}

		summary, err := runs.GetStatusCommand{
			Client:  c.Client,
			BaseURL: c.BaseURL,
			RunID:   srvResp.RunID,
		}.Run(ctx)
		if err != nil {
			return domaintypes.RunSummary{}, fmt.Errorf("batch create: fetch run summary: %w", err)
		}

		return summary, nil
	}

	// Non-success: read error body and return error.
	return domaintypes.RunSummary{}, httpx.WrapError("batch create", resp.Status, resp.Body)
}

// ListBatchesCommand lists batch runs from the control plane.
type ListBatchesCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Limit   int32 // Max results to return (default 50, max 100).
	Offset  int32 // Number of results to skip.
}

// Run executes GET /v1/runs to list batch runs with pagination.
func (c ListBatchesCommand) Run(ctx context.Context) ([]domaintypes.RunSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("batch list: %w", err)
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
	endpoint.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("batch list: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("batch list: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("batch list", resp.Status, resp.Body)
	}

	// Response structure: {"runs": [...]}
	var result struct {
		Runs []domaintypes.RunSummary `json:"runs"`
	}
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("batch list: decode response: %w", err)
	}

	return result.Runs, nil
}
