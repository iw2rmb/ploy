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
	"time"
)

// BatchSummary represents a batch run summary from the control-plane.
// It mirrors the server's RunBatchSummary type for CLI consumption.
type BatchSummary struct {
	ID         string         `json:"id"`
	Name       *string        `json:"name,omitempty"`
	Status     string         `json:"status"`
	RepoURL    string         `json:"repo_url"`
	BaseRef    string         `json:"base_ref"`
	TargetRef  string         `json:"target_ref"`
	CreatedBy  *string        `json:"created_by,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Counts     *RunRepoCounts `json:"repo_counts,omitempty"`
}

// RunRepoCounts aggregates the count of repos by status within a batch.
// DerivedStatus provides a single batch-level status derived from repo states.
type RunRepoCounts struct {
	Total         int32  `json:"total"`
	Pending       int32  `json:"pending"`
	Running       int32  `json:"running"`
	Succeeded     int32  `json:"succeeded"`
	Failed        int32  `json:"failed"`
	Skipped       int32  `json:"skipped"`
	Cancelled     int32  `json:"cancelled"`
	DerivedStatus string `json:"derived_status"`
}

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
		RepoURL:   c.RepoURL,
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
			TicketID  string `json:"run_id"`
			Status    string `json:"status"`
			RepoURL   string `json:"repo_url"`
			BaseRef   string `json:"base_ref"`
			TargetRef string `json:"target_ref"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&srvResp); err != nil {
			return BatchSummary{}, fmt.Errorf("batch create: decode response: %w", err)
		}
		return BatchSummary{
			ID:        srvResp.TicketID,
			Name:      c.Name,
			Status:    strings.ToLower(srvResp.Status),
			RepoURL:   srvResp.RepoURL,
			BaseRef:   srvResp.BaseRef,
			TargetRef: srvResp.TargetRef,
			CreatedBy: c.CreatedBy,
			CreatedAt: time.Now(),
		}, nil
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

// GetBatchStatusCommand retrieves detailed status for a single batch run.
type GetBatchStatusCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	BatchID string // UUID of the batch run.
}

// Run executes GET /v1/runs/{id} to fetch batch run details.
func (c GetBatchStatusCommand) Run(ctx context.Context) (BatchSummary, error) {
	if c.Client == nil {
		return BatchSummary{}, fmt.Errorf("batch status: http client required")
	}
	if c.BaseURL == nil {
		return BatchSummary{}, fmt.Errorf("batch status: base url required")
	}
	if strings.TrimSpace(c.BatchID) == "" {
		return BatchSummary{}, fmt.Errorf("batch status: batch id required")
	}

	endpoint := c.BaseURL.JoinPath("/v1/runs", strings.TrimSpace(c.BatchID))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return BatchSummary{}, fmt.Errorf("batch status: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return BatchSummary{}, fmt.Errorf("batch status: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return BatchSummary{}, decodeHTTPError(resp, "batch status")
	}

	var summary BatchSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return BatchSummary{}, fmt.Errorf("batch status: decode response: %w", err)
	}

	return summary, nil
}

// StopBatchCommand stops a batch run and cancels all pending repos.
type StopBatchCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	BatchID string // UUID of the batch run to stop.
}

// Run executes POST /v1/runs/{id}/stop to stop the batch run.
// Returns the updated batch summary on success.
func (c StopBatchCommand) Run(ctx context.Context) (BatchSummary, error) {
	if c.Client == nil {
		return BatchSummary{}, fmt.Errorf("batch stop: http client required")
	}
	if c.BaseURL == nil {
		return BatchSummary{}, fmt.Errorf("batch stop: base url required")
	}
	if strings.TrimSpace(c.BatchID) == "" {
		return BatchSummary{}, fmt.Errorf("batch stop: batch id required")
	}

	endpoint := c.BaseURL.JoinPath("/v1/runs", strings.TrimSpace(c.BatchID), "stop")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return BatchSummary{}, fmt.Errorf("batch stop: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return BatchSummary{}, fmt.Errorf("batch stop: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return BatchSummary{}, decodeHTTPError(resp, "batch stop")
	}

	var summary BatchSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return BatchSummary{}, fmt.Errorf("batch stop: decode response: %w", err)
	}

	return summary, nil
}

// StartBatchCommand starts execution for pending repos in a batch run.
type StartBatchCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	BatchID string // UUID of the batch run to start.
}

// StartBatchResult contains the result of starting a batch run.
type StartBatchResult struct {
	RunID       string `json:"run_id"`
	Started     int    `json:"started"`      // Number of repos that started.
	AlreadyDone int    `json:"already_done"` // Number of repos in terminal state.
	Pending     int    `json:"pending"`      // Number of repos still pending.
}

// Run executes POST /v1/runs/{id}/start to start execution for pending repos.
func (c StartBatchCommand) Run(ctx context.Context) (StartBatchResult, error) {
	if c.Client == nil {
		return StartBatchResult{}, fmt.Errorf("batch start: http client required")
	}
	if c.BaseURL == nil {
		return StartBatchResult{}, fmt.Errorf("batch start: base url required")
	}
	if strings.TrimSpace(c.BatchID) == "" {
		return StartBatchResult{}, fmt.Errorf("batch start: batch id required")
	}

	endpoint := c.BaseURL.JoinPath("/v1/runs", strings.TrimSpace(c.BatchID), "start")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return StartBatchResult{}, fmt.Errorf("batch start: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return StartBatchResult{}, fmt.Errorf("batch start: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return StartBatchResult{}, decodeHTTPError(resp, "batch start")
	}

	var result StartBatchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StartBatchResult{}, fmt.Errorf("batch start: decode response: %w", err)
	}

	return result, nil
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
