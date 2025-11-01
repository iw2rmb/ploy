package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// InspectCommand prints a summary for a specific job scoped by ticket.
type InspectCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Ticket  string
	JobID   string
	Output  io.Writer
}

// Run performs GET /v1/jobs/{id}?ticket=...
func (c InspectCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("jobs inspect: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("jobs inspect: base url required")
	}
	job := strings.TrimSpace(c.JobID)
	if job == "" {
		return errors.New("jobs inspect: job id required")
	}
	ticket := strings.TrimSpace(c.Ticket)
	if ticket == "" {
		return errors.New("jobs inspect: ticket required")
	}

	endpoint, err := url.Parse(c.BaseURL.String())
	if err != nil {
		return err
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/v1/jobs/" + url.PathEscape(job)
	q := endpoint.Query()
	q.Set("ticket", ticket)
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("jobs inspect: %s", msg)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if c.Output != nil {
		id := strings.TrimSpace(asString(payload["id"]))
		state := strings.TrimSpace(asString(payload["state"]))
		step := strings.TrimSpace(asString(payload["step_id"]))
		_, _ = fmt.Fprintf(c.Output, "Job %s: %s step=%s\n", id, state, step)
	}
	return nil
}
