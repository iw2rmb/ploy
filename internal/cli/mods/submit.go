package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// SubmitCommand submits a Mods run to the control plane.
// The command submits a single-repo run via POST /v1/runs, then fetches the
// canonical Mods-style RunSummary via GET /v1/runs/{id}/status for display.
type SubmitCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Request modsapi.RunSubmitRequest
}

// Run executes the submission against the control plane endpoint.
// POST /v1/runs returns 201 Created with {run_id, mod_id, spec_id}.
// GET /v1/runs/{id}/status returns the canonical RunSummary.
func (c SubmitCommand) Run(ctx context.Context) (modsapi.RunSummary, error) {
	if c.Client == nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: http client required")
	}
	if c.BaseURL == nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: base url required")
	}

	reqBody := c.Request
	reqBody.RepoURL = strings.TrimSpace(reqBody.RepoURL)
	if err := domaintypes.RepoURL(reqBody.RepoURL).Validate(); err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: repo_url: %w", err)
	}
	reqBody.BaseRef = strings.TrimSpace(reqBody.BaseRef)
	if err := domaintypes.GitRef(reqBody.BaseRef).Validate(); err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: base_ref: %w", err)
	}
	reqBody.TargetRef = strings.TrimSpace(reqBody.TargetRef)
	if err := domaintypes.GitRef(reqBody.TargetRef).Validate(); err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: target_ref: %w", err)
	}
	if len(reqBody.Spec) == 0 {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: spec is required")
	}

	// Control-plane submission endpoint: POST /v1/runs
	endpoint := c.BaseURL.ResolveReference(&url.URL{Path: "/v1/runs"})

	// Marshal the canonical submit request.
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Server returns 201 Created with {run_id, mod_id, spec_id}.
	if resp.StatusCode == http.StatusCreated {
		var created struct {
			RunID string `json:"run_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
			return modsapi.RunSummary{}, fmt.Errorf("mods submit: decode response: %w", err)
		}
		runID := domaintypes.RunID(strings.TrimSpace(created.RunID))
		if runID.IsZero() {
			return modsapi.RunSummary{}, fmt.Errorf("mods submit: empty run_id in response")
		}
		return fetchRunSummary(ctx, c.BaseURL, c.Client, runID)
	}

	// Handle error responses.
	var apiErr struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&apiErr)
	message := strings.TrimSpace(apiErr.Error)
	if message == "" {
		message = resp.Status
	}
	return modsapi.RunSummary{}, fmt.Errorf("mods submit: %s", message)
}

func fetchRunSummary(ctx context.Context, baseURL *url.URL, httpClient *http.Client, runID domaintypes.RunID) (modsapi.RunSummary, error) {
	endpoint := baseURL.ResolveReference(&url.URL{Path: "/v1/runs/" + url.PathEscape(runID.String()) + "/status"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: build status request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: http status request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		message := strings.TrimSpace(apiErr.Error)
		if message == "" {
			message = resp.Status
		}
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: %s", message)
	}
	var summary modsapi.RunSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: decode status response: %w", err)
	}
	summary.RunID = runID
	return summary, nil
}
