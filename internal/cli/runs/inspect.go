package runs

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
	JobID   string
	Output  io.Writer
}

// Run performs GET /v1/mods/{id}
func (c InspectCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("runs inspect: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("runs inspect: base url required")
	}
	job := strings.TrimSpace(c.JobID)
	if job == "" {
		return errors.New("runs inspect: run id required")
	}
	endpoint, err := url.Parse(c.BaseURL.String())
	if err != nil {
		return err
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/v1/mods/" + url.PathEscape(job)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
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
		return fmt.Errorf("runs inspect: %s", msg)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if c.Output != nil {
		id := toString(payload["ticket_id"])
		status := toString(payload["status"])
		step := toString(payload["step_id"]) // may be empty
		_, _ = fmt.Fprintf(c.Output, "Ticket %s: %s step=%s\n", id, status, step)
	}
	return nil
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		if t == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(t))
	}
}
