package grid

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func (c *Client) collectEvidence(ctx context.Context, runID string, term terminalRun) *runner.StageEvidence {
	trimmedRunID := strings.TrimSpace(runID)
	if trimmedRunID == "" {
		return nil
	}
	if c.controlHTTP == nil || c.controlStatus == nil {
		return nil
	}
	httpClient, err := c.controlHTTP(ctx)
	if err != nil || httpClient == nil {
		return nil
	}
	status := c.controlStatus()
	base := strings.TrimRight(strings.TrimSpace(status.APIEndpoint), "/")
	if base == "" {
		return nil
	}

	evidence := &runner.StageEvidence{
		Metadata: copyStringMap(term.metadata),
		Result:   cloneAnyMap(term.result),
	}

	jobInfo, err := fetchJobStatus(ctx, httpClient, base, trimmedRunID)
	if err == nil && jobInfo != nil {
		state := strings.TrimSpace(jobInfo.State)
		if state != "" {
			evidence.JobState = state
		}
		if jobInfo.ExitCode != nil {
			exit := *jobInfo.ExitCode
			evidence.ExitCode = &exit
		}
		if len(jobInfo.TerminalLog) > 0 && strings.TrimSpace(evidence.LogTail) == "" {
			evidence.LogTail = string(jobInfo.TerminalLog)
			evidence.Source = "terminal_log"
		}
		if len(jobInfo.Metadata) > 0 {
			if evidence.Metadata == nil {
				evidence.Metadata = make(map[string]string, len(jobInfo.Metadata))
			}
			for key, value := range jobInfo.Metadata {
				evidence.Metadata[key] = value
			}
		}
	}

	events, err := fetchJobEvents(ctx, httpClient, base, trimmedRunID, 200)
	if err == nil && len(events) > 0 {
		evidence.Events = make([]runner.StageEvidenceEvent, 0, len(events))
		for _, evt := range events {
			entry := runner.StageEvidenceEvent{
				Type:  strings.TrimSpace(evt.Type),
				State: strings.TrimSpace(evt.Job.State),
				Time:  evt.Time,
			}
			if evt.Job.ExitCode != nil {
				exit := *evt.Job.ExitCode
				entry.ExitCode = &exit
			}
			entry.Reason = strings.TrimSpace(evt.Job.Reason)
			evidence.Events = append(evidence.Events, entry)
		}
	}

	if logs, err := fetchLogs(ctx, httpClient, base, trimmedRunID, c.logTail); err == nil {
		if trimmed := strings.TrimSpace(logs); trimmed != "" {
			evidence.LogTail = logs
			evidence.Source = "control_plane_logs"
		}
	}

	if evidence.JobState == "" && term.status != "" {
		evidence.JobState = strings.TrimSpace(strings.ToLower(string(term.status)))
	}
	if evidence.JobState == "" {
		evidence.JobState = "unknown"
	}

	hasLog := strings.TrimSpace(evidence.LogTail) != ""
	hasEvents := len(evidence.Events) > 0
	hasMetadata := len(evidence.Metadata) > 0
	hasResult := len(evidence.Result) > 0
	hasState := strings.TrimSpace(evidence.JobState) != "" && !strings.EqualFold(evidence.JobState, "unknown")
	hasExit := evidence.ExitCode != nil
	if !hasLog && !hasEvents && !hasMetadata && !hasResult && !hasState && !hasExit {
		return nil
	}
	return evidence
}

func fetchLogs(ctx context.Context, httpClient *http.Client, base, runID string, tail int) (result string, err error) {
	if tail <= 0 {
		tail = 200
	}
	url := fmt.Sprintf("%s/v1/workflows/jobs/%s/logs?tail=%d", base, runID, tail)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("logs returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func fetchJobStatus(ctx context.Context, httpClient *http.Client, base, runID string) (status *jobStatus, err error) {
	url := fmt.Sprintf("%s/v1/workflows/jobs/%s", base, runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("job status returned %d", resp.StatusCode)
	}
	var payload jobStatus
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func fetchJobEvents(ctx context.Context, httpClient *http.Client, base, runID string, limit int) (events []jobEvent, err error) {
	if limit <= 0 {
		limit = 20
	}
	url := fmt.Sprintf("%s/v1/workflows/jobs/%s/events?limit=%d", base, runID, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("job events returned %d", resp.StatusCode)
	}
	var payload struct {
		Events []jobEvent `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Events, nil
}

type jobStatus struct {
	State       string            `json:"state"`
	ExitCode    *int              `json:"exit_code"`
	Reason      string            `json:"reason"`
	Metadata    map[string]string `json:"metadata"`
	TerminalLog []byte            `json:"terminal_log"`
}

type jobEvent struct {
	Type string        `json:"type"`
	Time time.Time     `json:"time"`
	Job  jobEventEntry `json:"job"`
}

type jobEventEntry struct {
	State       string `json:"state"`
	ExitCode    *int   `json:"exit_code"`
	Reason      string `json:"reason"`
	TerminalLog []byte `json:"terminal_log"`
}
