package mods

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ResumeCommand requests a resume for a Mods ticket via POST /v1/mods/{ticket}/resume.
// The server resume endpoint requeues eligible jobs for failed or canceled tickets,
// enabling continuation of a previously interrupted workflow.
type ResumeCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Ticket  string
	Output  io.Writer
}

// Run executes POST /v1/mods/{ticket}/resume and handles server responses:
//   - 202 Accepted: resume successfully initiated
//   - 200 OK: ticket already running (idempotent) or all jobs succeeded
//   - 404 Not Found: ticket does not exist
//   - 409 Conflict: ticket state not resumable (e.g., succeeded)
//   - 400 Bad Request: invalid ticket ID
func (c ResumeCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("mods resume: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("mods resume: base url required")
	}
	ticket := strings.TrimSpace(c.Ticket)
	if ticket == "" {
		return errors.New("mods resume: ticket required")
	}

	// Build the server resume endpoint URL.
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "mods", url.PathEscape(ticket), "resume")
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
			// Ticket ID does not exist in the control plane.
			if msg == "" {
				msg = "ticket not found"
			}
			return fmt.Errorf("mods resume: %s", msg)
		case http.StatusConflict:
			// Ticket state is not resumable (e.g., already succeeded).
			if msg == "" {
				msg = "ticket cannot be resumed"
			}
			return fmt.Errorf("mods resume: %s", msg)
		case http.StatusBadRequest:
			// Invalid ticket ID format.
			if msg == "" {
				msg = "invalid ticket id"
			}
			return fmt.Errorf("mods resume: %s", msg)
		default:
			// Generic error fallback for unexpected status codes.
			if msg == "" {
				msg = resp.Status
			}
			return fmt.Errorf("mods resume: %s", msg)
		}
	}

	// Success: resume was accepted or ticket is already running (idempotent).
	if c.Output != nil {
		_, _ = io.WriteString(c.Output, "Resume requested\n")
	}
	return nil
}
