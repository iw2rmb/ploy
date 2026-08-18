package runs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// ListRunDiffsCommand fetches the diff listing for a run.
// It returns structured data suitable for machine consumption (e.g., TUI).
type ListRunDiffsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
}

// Run executes GET /v1/runs/{run_id}/diffs and returns structured diffs.
func (c ListRunDiffsCommand) Run(ctx context.Context) (domainapi.DiffListResponse, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domainapi.DiffListResponse{}, fmt.Errorf("list run diffs: %w", err)
	}
	if c.RunID.IsZero() {
		return domainapi.DiffListResponse{}, errors.New("list run diffs: run id required")
	}
	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "diffs")
	result, err := httpx.DoJSON[domainapi.DiffListResponse](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "list run diffs")
	if err != nil {
		return domainapi.DiffListResponse{}, err
	}
	if result.Diffs == nil {
		result.Diffs = []domainapi.DiffListItem{}
	}
	return result, nil
}

// RunDiffsCommand lists diffs for a specific run and optionally downloads the newest patch.
type RunDiffsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed)
	Output  io.Writer

	Download bool   // when true, download newest diff and print to stdout (gunzipped)
	SavePath string // optional path to save the gunzipped patch
}

// Run executes the command.
func (c RunDiffsCommand) Run(ctx context.Context) error {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return fmt.Errorf("run diffs: %w", err)
	}
	if c.RunID.IsZero() {
		return errors.New("run diffs: run id required")
	}
	runID := c.RunID.String()
	out := c.Output
	if out == nil {
		out = io.Discard
	}

	// List diffs via run endpoint
	listURL := c.BaseURL.JoinPath("v1", "runs", runID, "diffs")
	listing, err := httpx.DoJSON[domainapi.DiffListResponse](ctx, c.Client, http.MethodGet, listURL.String(), nil, http.StatusOK, "run diffs")
	if err != nil {
		return err
	}

	if !c.Download {
		for _, d := range listing.Diffs {
			job := strings.TrimSpace(d.JobID.String())
			if job == "" {
				job = "-"
			}
			_, _ = fmt.Fprintf(out, "%s job=%s size=%d created=%s\n", d.ID.String(), job, d.Size, d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		return nil
	}

	if len(listing.Diffs) == 0 {
		return errors.New("run diffs: no diffs available for this run")
	}
	// Newest first by API; take first.
	diffID := listing.Diffs[0].ID

	dlURL := c.BaseURL.JoinPath("v1", "runs", runID, "diffs")
	q := dlURL.Query()
	q.Set("download", "true")
	q.Set("diff_id", diffID.String())
	dlURL.RawQuery = q.Encode()
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL.String(), nil)
	if err != nil {
		return err
	}
	resp2, err := c.Client.Do(req2)
	if err != nil {
		return err
	}
	defer httpx.DrainAndClose(resp2)
	if resp2.StatusCode != http.StatusOK {
		return httpx.WrapError("run diffs", resp2.Status, resp2.Body)
	}
	patch, err := httpx.GunzipToBytes(io.LimitReader(resp2.Body, httpx.MaxDownloadBodyBytes), httpx.MaxGunzipOutputBytes)
	if err != nil {
		return fmt.Errorf("gunzip patch: %w", err)
	}

	if strings.TrimSpace(c.SavePath) != "" {
		// ensure dir exists
		_ = os.MkdirAll(filepath.Dir(c.SavePath), 0o750)
		if err := os.WriteFile(c.SavePath, patch, 0o600); err != nil {
			return fmt.Errorf("write patch: %w", err)
		}
		_, _ = fmt.Fprintf(out, "Saved diff to %s (%d bytes)\n", c.SavePath, len(patch))
		return nil
	}

	// print to stdout
	_, _ = out.Write(patch)
	if len(patch) == 0 || patch[len(patch)-1] != '\n' {
		_, _ = out.Write([]byte("\n"))
	}
	return nil
}
