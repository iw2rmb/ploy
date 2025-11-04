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
	// Spec, when non-empty, is forwarded to the simplified submit payload
	// for servers that accept repo_url/base_ref/target_ref (+optional spec).
	Spec []byte
}

// Run executes the submission against the control plane endpoint.
func (c SubmitCommand) Run(ctx context.Context) (modsapi.TicketSummary, error) {
	if c.Client == nil {
		return modsapi.TicketSummary{}, fmt.Errorf("mods submit: http client required")
	}
	if c.BaseURL == nil {
		return modsapi.TicketSummary{}, fmt.Errorf("mods submit: base url required")
	}
	// Control-plane submission endpoint: POST /v1/mods
	// Keep request shape compatible with existing server/tests (modsapi.TicketSubmitRequest).
	// The server may introduce a simplified 201 flow, but we continue to send the
	// canonical request for backward compatibility and map 201 responses below.
	endpoint := c.BaseURL.ResolveReference(&url.URL{Path: "/v1/mods"})

	// Marshal the canonical submit request as-is.
	payload, err := json.Marshal(c.Request)
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
		// Fallback: some servers expect a simplified payload (repo_url/base_ref/target_ref).
		// If the first attempt failed with a client error OR when a Spec is present,
		// try the simplified shape once including Spec when provided.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 || len(c.Spec) > 0 {
			// Drain body to allow connection reuse.
			_ = json.NewDecoder(resp.Body).Decode(&struct{}{})

			// Build simplified payload from the canonical request fields if available.
			repoURL := strings.TrimSpace(c.Request.Repository)
			baseRef := strings.TrimSpace(c.Request.Metadata["repo_base_ref"])
			targetRef := strings.TrimSpace(c.Request.Metadata["repo_target_ref"])
			if repoURL != "" && baseRef != "" && targetRef != "" {
				var specPtr *json.RawMessage
				if len(c.Spec) > 0 {
					jr := json.RawMessage(append([]byte(nil), c.Spec...))
					specPtr = &jr
				}
				simple := struct {
					RepoURL   string           `json:"repo_url"`
					BaseRef   string           `json:"base_ref"`
					TargetRef string           `json:"target_ref"`
					Spec      *json.RawMessage `json:"spec,omitempty"`
				}{RepoURL: repoURL, BaseRef: baseRef, TargetRef: targetRef, Spec: specPtr}
				payload2, err2 := json.Marshal(simple)
				if err2 == nil {
					req2, err2 := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload2))
					if err2 == nil {
						req2.Header.Set("Content-Type", "application/json")
						if resp2, err2 := c.Client.Do(req2); err2 == nil {
							defer func() { _ = resp2.Body.Close() }()
							if resp2.StatusCode == http.StatusCreated {
								var srvResp struct {
									TicketID  string `json:"ticket_id"`
									Status    string `json:"status"`
									RepoURL   string `json:"repo_url"`
									BaseRef   string `json:"base_ref"`
									TargetRef string `json:"target_ref"`
								}
								if err := json.NewDecoder(resp2.Body).Decode(&srvResp); err != nil {
									return modsapi.TicketSummary{}, fmt.Errorf("mods submit: decode response: %w", err)
								}
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
							}
						}
					}
				}
			}
		}

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
