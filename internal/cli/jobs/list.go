package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// ListCommand lists jobs for a ticket.
type ListCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Ticket  string
	Output  io.Writer
}

// Run performs GET /v1/jobs?ticket=... and prints one line per job.
func (c ListCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("jobs ls: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("jobs ls: base url required")
	}
	ticket := strings.TrimSpace(c.Ticket)
	if ticket == "" {
		return errors.New("jobs ls: ticket required")
	}
	endpoint, err := url.Parse(c.BaseURL.String())
	if err != nil {
		return err
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/v1/jobs"
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
		return fmt.Errorf("jobs ls: %s", msg)
	}
	var payload struct {
		Jobs []struct {
			ID, StepID, State string `json:"id" json2:"step_id"`
		} `json:"jobs"`
	}
	// decode into generic map then pick fields to avoid brittle tags
	var raw struct {
		Jobs []map[string]any `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return err
	}
	for _, j := range raw.Jobs {
		id := strings.TrimSpace(asString(j["id"]))
		step := strings.TrimSpace(asString(j["step_id"]))
		state := strings.TrimSpace(asString(j["state"]))
		payload.Jobs = append(payload.Jobs, struct {
			ID, StepID, State string `json:"id" json2:"step_id"`
		}{ID: id, StepID: step, State: state})
	}
	if c.Output == nil {
		return nil
	}
	// Stable order by ID for output determinism in tests
	sort.Slice(payload.Jobs, func(i, j int) bool { return payload.Jobs[i].ID < payload.Jobs[j].ID })
	for _, j := range payload.Jobs {
		_, _ = fmt.Fprintf(c.Output, "%s %s %s\n", j.ID, j.StepID, j.State)
	}
	return nil
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}
