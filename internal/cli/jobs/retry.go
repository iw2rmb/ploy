package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// RetryCommand requests a retry for a failed job.
type RetryCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Ticket  string
	JobID   string
	Output  io.Writer
}

// Run performs POST /v1/jobs/{id}/retry?ticket=...
func (c RetryCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("jobs retry: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("jobs retry: base url required")
	}
	job := strings.TrimSpace(c.JobID)
	if job == "" {
		return errors.New("jobs retry: job id required")
	}
	ticket := strings.TrimSpace(c.Ticket)
	if ticket == "" {
		return errors.New("jobs retry: ticket required")
	}
	endpoint, err := url.Parse(c.BaseURL.String())
	if err != nil {
		return err
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/v1/jobs/" + url.PathEscape(job) + "/retry"
	q := endpoint.Query()
	q.Set("ticket", ticket)
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("jobs retry: %s", msg)
	}
	if c.Output != nil {
		_, _ = io.WriteString(c.Output, "Retry requested\n")
	}
	return nil
}
