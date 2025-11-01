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
	// New control-plane submission endpoint (3.1): POST /v1/mods
	endpoint := c.BaseURL.ResolveReference(&url.URL{Path: "/v1/mods"})

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

	if resp.StatusCode != http.StatusAccepted {
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

	var submitResp modsapi.TicketSubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		return modsapi.TicketSummary{}, fmt.Errorf("mods submit: decode response: %w", err)
	}
	return submitResp.Ticket, nil
}
