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
// The command uses a single canonical submit contract: POST /v1/mods returns
// 201 Created with a RunSummary response containing run_id, state, and metadata.
type SubmitCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Request modsapi.RunSubmitRequest
}

// Run executes the submission against the control plane endpoint.
// POST /v1/mods returns 201 Created with a canonical RunSummary response.
// This is the single canonical submit contract — no fallback to 202 or alternative
// payload shapes. Error responses return an error with the server message.
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
	// Control-plane submission endpoint: POST /v1/mods
	endpoint := c.BaseURL.ResolveReference(&url.URL{Path: "/v1/mods"})

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

	// Server returns 201 Created with the canonical RunSummary response.
	if resp.StatusCode == http.StatusCreated {
		var summary modsapi.RunSummary
		if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
			return modsapi.RunSummary{}, fmt.Errorf("mods submit: decode response: %w", err)
		}
		// Ensure RunID is normalised to the domain type.
		summary.RunID = domaintypes.RunID(strings.TrimSpace(string(summary.RunID)))
		return summary, nil
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
