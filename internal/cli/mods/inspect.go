package mods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// InspectCommand fetches and prints ticket summary.
type InspectCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Ticket  string
	Output  io.Writer
}

// Run performs GET /v1/mods/{ticket} and prints a one-line summary.
func (c InspectCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("mods inspect: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("mods inspect: base url required")
	}
	ticket := strings.TrimSpace(c.Ticket)
	if ticket == "" {
		return errors.New("mods inspect: ticket required")
	}
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "mods", url.PathEscape(ticket))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("mods inspect: %s", msg)
	}
	var payload modsapi.TicketStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if c.Output != nil {
		_, _ = fmt.Fprintf(c.Output, "Ticket %s: %s\n", strings.TrimSpace(payload.Ticket.TicketID), strings.ToLower(string(payload.Ticket.State)))
		if mrURL, ok := payload.Ticket.Metadata["mr_url"]; ok && mrURL != "" {
			_, _ = fmt.Fprintf(c.Output, "MR: %s\n", mrURL)
		}
	}
	return nil
}
