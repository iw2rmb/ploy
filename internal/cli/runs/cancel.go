package runs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// CancelCommand requests cancellation for a run.
type CancelCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return err
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
