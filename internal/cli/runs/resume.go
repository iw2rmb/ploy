package runs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// ResumeCommand requests a resume for a run via POST /v1/runs/{id}/resume.
// The server resume endpoint requeues eligible jobs for failed or canceled runs,
// enabling continuation of a previously interrupted workflow.
type ResumeCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	Output  io.Writer
}

// Run executes POST /v1/runs/{id}/resume and handles server responses:
//   - 202 Accepted: resume successfully initiated
//   - 200 OK: run already running (idempotent) or all jobs succeeded
//   - 404 Not Found: run does not exist
//   - 409 Conflict: run state not resumable (e.g., succeeded)
//   - 400 Bad Request: invalid run ID
func (c ResumeCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("runs resume: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("runs resume: base url required")
	}
	if c.RunID.IsZero() {
		return errors.New("runs resume: run id required")
	}
	runID := c.RunID.String()

	// Build the server resume endpoint URL.
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "runs", url.PathEscape(runID), "resume")
	if err != nil {
		return err
	}

	// Create and execute the POST request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle non-success responses with status-specific error messages.
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(data))

		// Provide user-friendly error messages based on HTTP status codes.
		switch resp.StatusCode {
		case http.StatusNotFound:
			// Run ID does not exist in the control plane.
			if msg == "" {
				msg = "run not found"
			}
			return fmt.Errorf("runs resume: %s", msg)
		case http.StatusConflict:
			// Run state is not resumable (e.g., already succeeded).
			if msg == "" {
				msg = "run cannot be resumed"
			}
			return fmt.Errorf("runs resume: %s", msg)
		case http.StatusBadRequest:
			// Invalid run ID format.
			if msg == "" {
				msg = "invalid run id"
			}
			return fmt.Errorf("runs resume: %s", msg)
		default:
			// Generic error fallback for unexpected status codes.
			if msg == "" {
				msg = resp.Status
			}
			return fmt.Errorf("runs resume: %s", msg)
		}
	}

	// Success: resume was accepted or run is already running (idempotent).
	if c.Output != nil {
		_, _ = io.WriteString(c.Output, "Resume requested\n")
	}
	return nil
}
