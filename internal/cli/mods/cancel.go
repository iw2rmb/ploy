package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// CancelCommand requests cancellation for a Mods run.
type CancelCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   string
	Reason  string
	Output  io.Writer
}

// Run executes the cancel request (POST /v1/mods/{id}/cancel).
func (c CancelCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("mods cancel: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("mods cancel: base url required")
	}
	runID := strings.TrimSpace(c.RunID)
	if runID == "" {
		return errors.New("mods cancel: run id required")
	}
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "mods", url.PathEscape(runID), "cancel")
	if err != nil {
		return err
	}
	payload := map[string]string{}
	if r := strings.TrimSpace(c.Reason); r != "" {
		payload["reason"] = r
	}
	var body io.Reader
	if len(payload) > 0 {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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
		return fmt.Errorf("mods cancel: %s", msg)
	}
	if c.Output != nil {
		_, _ = io.WriteString(c.Output, "Cancellation requested\n")
	}
	return nil
}
