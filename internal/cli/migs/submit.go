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
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
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
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: %w", err)
	}

	reqBody := c.Request
	reqBody.RepoURL = domaintypes.RepoURL(strings.TrimSpace(reqBody.RepoURL.String()))
	if err := reqBody.RepoURL.Validate(); err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: repo_url: %w", err)
	}
	reqBody.BaseRef = domaintypes.GitRef(strings.TrimSpace(reqBody.BaseRef.String()))
	if err := reqBody.BaseRef.Validate(); err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: base_ref: %w", err)
	}
	reqBody.TargetRef = domaintypes.GitRef(strings.TrimSpace(reqBody.TargetRef.String()))
	if err := reqBody.TargetRef.Validate(); err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: target_ref: %w", err)
	}
	if len(reqBody.Spec) == 0 {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: spec is required")
	}

	// Control-plane submission endpoint: POST /v1/runs
	endpoint := c.BaseURL.JoinPath("v1", "runs")

	// Marshal the canonical submit request.
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	// Server returns 201 Created with {run_id, mod_id, spec_id}.
	if resp.StatusCode == http.StatusCreated {
		var created struct {
			RunID  string `json:"run_id"`
			MigID  string `json:"mig_id"`
			SpecID string `json:"spec_id"`
		}
		if err := httpx.DecodeResponseJSON(resp.Body, &created, httpx.MaxJSONBodyBytes); err != nil {
			return modsapi.RunSummary{}, fmt.Errorf("migs submit: decode response: %w", err)
		}
		runID := domaintypes.RunID(strings.TrimSpace(created.RunID))
		if runID.IsZero() {
			return modsapi.RunSummary{}, fmt.Errorf("migs submit: empty run_id in response")
		}
		return fetchRunSummary(ctx, c.BaseURL, c.Client, runID)
	}

	return modsapi.RunSummary{}, httpx.WrapError("migs submit", resp.Status, resp.Body)
}

func fetchRunSummary(ctx context.Context, baseURL *url.URL, httpClient *http.Client, runID domaintypes.RunID) (modsapi.RunSummary, error) {
	endpoint := baseURL.JoinPath("v1", "runs", runID.String(), "status")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: build status request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: http status request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return modsapi.RunSummary{}, httpx.WrapError("migs submit", resp.Status, resp.Body)
	}
	var summary modsapi.RunSummary
	if err := httpx.DecodeResponseJSON(resp.Body, &summary, httpx.MaxJSONBodyBytes); err != nil {
		return modsapi.RunSummary{}, fmt.Errorf("migs submit: decode status response: %w", err)
	}
	summary.RunID = runID
	return summary, nil
}
