package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// SubmitCommand submits a Mods ticket to the control plane.
type SubmitCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Request modsapi.TicketSubmitRequest
}

// Run executes the submission against the control plane endpoint.
func (c SubmitCommand) Run(ctx context.Context) (modsapi.TicketSummary, error) {
	if c.Client == nil {
		return modsapi.TicketSummary{}, fmt.Errorf("mods submit: http client required")
	}
	if c.BaseURL == nil {
		return modsapi.TicketSummary{}, fmt.Errorf("mods submit: base url required")
	}
	// New control-plane submission endpoint: POST /v1/mods (server expects simplified payload)
	endpoint := c.BaseURL.ResolveReference(&url.URL{Path: "/v1/mods"})

	// Transform the CLI request into server's simplified shape.
	type serverSubmit struct {
		RepoURL   string                 `json:"repo_url"`
		BaseRef   string                 `json:"base_ref"`
		TargetRef string                 `json:"target_ref"`
		CommitSha *string                `json:"commit_sha,omitempty"`
		Spec      map[string]interface{} `json:"spec,omitempty"`
		CreatedBy string                 `json:"created_by,omitempty"`
	}
	// Extract base/target refs and optional commit from metadata.
	baseRef := strings.TrimSpace(c.Request.Metadata["repo_base_ref"])
	targetRef := strings.TrimSpace(c.Request.Metadata["repo_target_ref"])
	var commit *string
	if v := strings.TrimSpace(c.Request.Metadata["repo_commit_sha"]); v != "" {
		commit = &v
	}
	srv := serverSubmit{
		RepoURL:   strings.TrimSpace(c.Request.Repository),
		BaseRef:   baseRef,
		TargetRef: targetRef,
		CommitSha: commit,
		Spec:      map[string]interface{}{},
		CreatedBy: strings.TrimSpace(c.Request.Submitter),
	}
	payload, err := json.Marshal(srv)
	if err != nil {
		return modsapi.TicketSummary{}, fmt.Errorf("mods submit: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return modsapi.TicketSummary{}, fmt.Errorf("mods submit: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return modsapi.TicketSummary{}, fmt.Errorf("mods submit: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusCreated: // 201 — server simplified summary
		var srvResp struct {
			TicketID  string `json:"ticket_id"`
			Status    string `json:"status"`
			RepoURL   string `json:"repo_url"`
			BaseRef   string `json:"base_ref"`
			TargetRef string `json:"target_ref"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&srvResp); err != nil {
			return modsapi.TicketSummary{}, fmt.Errorf("mods submit: decode response: %w", err)
		}
		// Map to modsapi summary type.
		return modsapi.TicketSummary{
			TicketID:   srvResp.TicketID,
			State:      modsapi.TicketState(strings.ToLower(strings.TrimSpace(srvResp.Status))),
			Repository: srvResp.RepoURL,
			Metadata: map[string]string{
				"repo_base_ref":   srvResp.BaseRef,
				"repo_target_ref": srvResp.TargetRef,
			},
			Stages: make(map[string]modsapi.StageStatus),
		}, nil
	case http.StatusAccepted: // 202 — legacy/alternate response shape still supported
		var submitResp modsapi.TicketSubmitResponse
		if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
			return modsapi.TicketSummary{}, fmt.Errorf("mods submit: decode response: %w", err)
		}
		return submitResp.Ticket, nil
	default:
		var apiErr struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		message := strings.TrimSpace(apiErr.Error)
		if message == "" {
			message = resp.Status
		}
		return modsapi.TicketSummary{}, fmt.Errorf("mods submit: %s", message)
	}
}
