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

// ResumeCommand requests a resume for a Mods ticket.
type ResumeCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Ticket  string
	Output  io.Writer
}

// Run executes POST /v1/mods/{ticket}/resume.
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
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "mods", url.PathEscape(ticket), "resume")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("mods resume: %s", msg)
	}
	if c.Output != nil {
		_, _ = io.WriteString(c.Output, "Resume requested\n")
	}
	return nil
}
