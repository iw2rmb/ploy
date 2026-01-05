// Package mods provides CLI client implementations for Mods operations.
// This file implements the batch run lifecycle client for create/list/stop/status
// operations against the control-plane /v1/runs endpoints.
//
// BatchClient encapsulates HTTP calls to the control-plane and maps responses
// to domain types for CLI consumption. It follows the same Command pattern
// used by SubmitCommand and InspectCommand.
package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// BatchSummary is an alias for the run-level Summary used by run commands.
// It mirrors the server's RunSummary type for CLI consumption.
// Uses domain type (RunID) for type-safe identification.
type BatchSummary = runs.Summary

// RunRepoCounts aggregates the count of repos by status within a batch.
// DerivedStatus provides a single batch-level status derived from repo states.
type RunRepoCounts = runs.RepoCounts

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

// Run executes POST /v1/mods to create a batch run with the shared spec.
// Returns the created batch summary on success.
func (c CreateBatchCommand) Run(ctx context.Context) (BatchSummary, error) {
	if c.Client == nil {
		return BatchSummary{}, fmt.Errorf("batch create: http client required")
	}
	if c.BaseURL == nil {
		return BatchSummary{}, fmt.Errorf("batch create: base url required")
	}

	repoURL := strings.TrimSpace(c.RepoURL)
	if err := domaintypes.RepoURL(repoURL).Validate(); err != nil {
		return BatchSummary{}, fmt.Errorf("batch create: repo_url: %w", err)
	}

	// Build the request payload matching server's expected format.
	// Server uses /v1/mods for initial creation with repo_url/base_ref/target_ref + spec.
	req := struct {
		Name      *string          `json:"name,omitempty"`
		RepoURL   string           `json:"repo_url"`
		BaseRef   string           `json:"base_ref"`
		TargetRef string           `json:"target_ref"`
		Spec      *json.RawMessage `json:"spec,omitempty"`
		CreatedBy *string          `json:"created_by,omitempty"`
	}{
		Name:      c.Name,
		RepoURL:   repoURL,
		BaseRef:   c.BaseRef,
		TargetRef: c.TargetRef,
		CreatedBy: c.CreatedBy,
	}

	if len(c.Spec) > 0 {
		spec := c.Spec
		req.Spec = &spec
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return BatchSummary{}, fmt.Errorf("batch create: marshal request: %w", err)
	}

	// POST /v1/mods to create the batch run.
	endpoint := c.BaseURL.JoinPath("/v1/mods")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return BatchSummary{}, fmt.Errorf("batch create: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return BatchSummary{}, fmt.Errorf("batch create: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle 201 Created response from server.
	if resp.StatusCode == http.StatusCreated {
		var srvResp struct {
			RunID domaintypes.RunID `json:"run_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&srvResp); err != nil {
			return BatchSummary{}, fmt.Errorf("batch create: decode response: %w", err)
		}

		summary, err := runs.GetStatusCommand{
			Client:  c.Client,
			BaseURL: c.BaseURL,
			RunID:   srvResp.RunID,
		}.Run(ctx)
		if err != nil {
			return BatchSummary{}, fmt.Errorf("batch create: fetch run summary: %w", err)
		}

		return summary, nil
	}

	// Non-success: read error body and return error.
	return BatchSummary{}, decodeHTTPError(resp, "batch create")
}

// ListBatchesCommand lists batch runs from the control plane.
type ListBatchesCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Limit   int32 // Max results to return (default 50, max 100).
	Offset  int32 // Number of results to skip.
}

// Run executes GET /v1/runs to list batch runs with pagination.
func (c ListBatchesCommand) Run(ctx context.Context) ([]BatchSummary, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("batch list: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("batch list: base url required")
	}

	// Build endpoint with query params.
	endpoint := c.BaseURL.JoinPath("/v1/runs")
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "batch list")
	}

	// Response structure: {"runs": [...]}
	var result struct {
		Runs []BatchSummary `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("batch list: decode response: %w", err)
	}

	return result.Runs, nil
}

// decodeHTTPError reads and formats an error from a non-success HTTP response.
// It attempts to extract an error message from the response body; otherwise,
// falls back to the HTTP status text.
func decodeHTTPError(resp *http.Response, prefix string) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("%s: %s", prefix, msg)
}
