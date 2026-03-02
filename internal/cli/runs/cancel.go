package runs

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

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// CancelCommand requests cancellation for a run.
type CancelCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	Reason  string
	Output  io.Writer
}

// Run executes the cancel request (POST /v1/runs/{id}/cancel).
func (c CancelCommand) Run(ctx context.Context) error {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return fmt.Errorf("runs cancel: %w", err)
	}
	if c.RunID.IsZero() {
		return errors.New("runs cancel: run id required")
	}
	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "cancel")
	payload := map[string]string{}
	if r := strings.TrimSpace(c.Reason); r != "" {
		payload["reason"] = r
	}
	var body io.Reader
	if len(payload) > 0 {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
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
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return httpx.WrapError("runs cancel", resp.Status, resp.Body)
	}
	if c.Output != nil {
		_, _ = io.WriteString(c.Output, "Cancellation requested\n")
	}
	return nil
}
