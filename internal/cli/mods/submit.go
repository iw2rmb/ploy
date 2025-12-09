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
type SubmitCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Request modsapi.RunSubmitRequest
	// Spec, when non-empty, is forwarded to the simplified submit payload
	// for servers that accept repo_url/base_ref/target_ref (+optional spec).
	Spec []byte
}

// Run executes the submission against the control plane endpoint.
// POST /v1/mods returns 201 with a canonical submit response (run_id, status, repo_url, etc.).
func (c SubmitCommand) Run(ctx context.Context) (modsapi.RunSummary, error) {
	if c.Client == nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: http client required")
	}
	if c.BaseURL == nil {
		return modsapi.RunSummary{}, fmt.Errorf("mods submit: base url required")
	}
	// Control-plane submission endpoint: POST /v1/mods
	endpoint := c.BaseURL.ResolveReference(&url.URL{Path: "/v1/mods"})

	// Marshal the canonical submit request.
	payload, err := json.Marshal(c.Request)
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

	// Server returns 201 Created with the canonical submit response.
	if resp.StatusCode == http.StatusCreated {
		var srvResp struct {
			RunID     string `json:"run_id"`
			Status    string `json:"status"`
			RepoURL   string `json:"repo_url"`
			BaseRef   string `json:"base_ref"`
			TargetRef string `json:"target_ref"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&srvResp); err != nil {
			return modsapi.RunSummary{}, fmt.Errorf("mods submit: decode response: %w", err)
		}
		// Map to modsapi.RunSummary.
		return modsapi.RunSummary{
			RunID:      domaintypes.RunID(srvResp.RunID),
			State:      modsapi.RunState(strings.ToLower(strings.TrimSpace(srvResp.Status))),
			Repository: srvResp.RepoURL,
			Metadata: map[string]string{
				"repo_base_ref":   srvResp.BaseRef,
				"repo_target_ref": srvResp.TargetRef,
			},
			Stages: make(map[string]modsapi.StageStatus),
		}, nil
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
