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

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// RepoDiffsCommand lists diffs for a specific repo execution within a run and
// optionally downloads the newest patch. This is the v1 repo-scoped version
// that replaces the legacy run-scoped DiffsCommand.
//
// Uses GET /v1/runs/{run_id}/repos/{repo_id}/diffs endpoint.
// Returns diffs filtered by repo_id via jobs.repo_id join.
type RepoDiffsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID     // Run ID (KSUID-backed)
	RepoID  domaintypes.MigRepoID // Repo ID (NanoID-backed)
	Output  io.Writer

	Download bool   // when true, download newest diff and print to stdout (gunzipped)
	SavePath string // optional path to save the gunzipped patch
}

// Run executes the command.
func (c RepoDiffsCommand) Run(ctx context.Context) error {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return fmt.Errorf("run repo diffs: %w", err)
	}
	if c.RunID.IsZero() {
		return errors.New("run repo diffs: run id required")
	}
	if c.RepoID.IsZero() {
		return errors.New("run repo diffs: repo id required")
	}
	runID := c.RunID.String()
	repoID := c.RepoID.String()
	out := c.Output
	if out == nil {
		out = io.Discard
	}

	// List diffs via repo-scoped endpoint
	listURL := c.BaseURL.JoinPath("v1", "runs", runID, "repos", repoID, "diffs")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return httpx.WrapError("run repo diffs", resp.Status, resp.Body)
	}
	var listing struct {
		Diffs []repoDiffEntry `json:"diffs"`
	}
	if err := httpx.DecodeJSON(resp.Body, &listing, httpx.MaxJSONBodyBytes); err != nil {
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
		return errors.New("run repo diffs: no diffs available for this repo execution")
	}
	// Newest first by API; take first.
	diffID := listing.Diffs[0].ID

	// Download gzipped patch via repo-scoped endpoint (download mode).
	dlURL := c.BaseURL.JoinPath("v1", "runs", runID, "repos", repoID, "diffs")
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
		return httpx.WrapError("run repo diffs", resp2.Status, resp2.Body)
	}
	patch, err := httpx.GunzipToBytes(io.LimitReader(resp2.Body, httpx.MaxDownloadBodyBytes), httpx.MaxGunzipOutputBytes)
	if err != nil {
		return fmt.Errorf("gunzip patch: %w", err)
	}

	if strings.TrimSpace(c.SavePath) != "" {
		// ensure dir exists
		_ = os.MkdirAll(filepath.Dir(c.SavePath), 0o755)
		if err := os.WriteFile(c.SavePath, patch, 0o644); err != nil {
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
