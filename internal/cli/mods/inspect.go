package mods

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
	var payload modsapi.RunStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if c.Output != nil {
		_, _ = fmt.Fprintf(c.Output, "Ticket %s: %s\n", strings.TrimSpace(string(payload.Ticket.RunID)), strings.ToLower(string(payload.Ticket.State)))
		if mrURL, ok := payload.Ticket.Metadata["mr_url"]; ok && mrURL != "" {
			_, _ = fmt.Fprintf(c.Output, "MR: %s\n", mrURL)
		}
		// Display build gate summary when available for quick gate health visibility.
		// The gate_summary reflects the final (post-mod) gate result when mods were executed,
		// or the pre-mod gate result if no mods ran. This ensures users see the authoritative
		// gate status at run completion. See stats.GateSummary() for priority logic.
		if gateSummary, ok := payload.Ticket.Metadata["gate_summary"]; ok && gateSummary != "" {
			_, _ = fmt.Fprintf(c.Output, "Gate: %s\n", gateSummary)
		}
		// Display job-level DAG state for visibility into gate/heal/re-gate workflow steps.
		// Jobs are sorted by StepIndex to reflect execution order in the healing DAG:
		//   pre-gate → mod-0 → post-gate
		//             │
		//             └─(fail)→ heal → re-gate → mod-0
		c.printJobGraph(payload.Ticket.Stages)
	}
	return nil
}

// printJobGraph outputs a summary of jobs sorted by step_index, showing job state.
// This surfaces the DAG structure visible via GET /v1/mods/{id} in human-readable form.
func (c InspectCommand) printJobGraph(stages map[string]modsapi.StageStatus) {
	if len(stages) == 0 {
		return
	}

	// Collect and sort jobs by StepIndex for execution-order display.
	type jobEntry struct {
		id     string
		status modsapi.StageStatus
	}
	jobs := make([]jobEntry, 0, len(stages))
	for id, s := range stages {
		jobs = append(jobs, jobEntry{id: id, status: s})
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].status.StepIndex < jobs[j].status.StepIndex
	})

	_, _ = fmt.Fprintln(c.Output, "Jobs:")
	for _, j := range jobs {
		// Format: "  [step_index] job_id: state"
		// The job_id is truncated to 8 chars for readability (KSUID prefix).
		shortID := j.id
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		_, _ = fmt.Fprintf(c.Output, "  [%d] %s: %s\n", j.status.StepIndex, shortID, strings.ToLower(string(j.status.State)))
	}
}
