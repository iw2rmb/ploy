package runs

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// DiffsCommand lists diffs for a run and optionally downloads the newest patch.
// Uses domain type (RunID) for type-safe identification.
type DiffsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed)
	Output  io.Writer

	Download bool   // when true, download newest diff and print to stdout (gunzipped)
	SavePath string // optional path to save the gunzipped patch
}

// Run executes the command.
func (c DiffsCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("run diffs: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("run diffs: base url required")
	}
	// Use domain type's IsZero method for validation.
	if c.RunID.IsZero() {
		return errors.New("run diffs: run id required")
	}
	runID := c.RunID.String()
	out := c.Output
	if out == nil {
		out = io.Discard
	}

	// List diffs
	listURL, err := url.JoinPath(c.BaseURL.String(), "v1", "runs", url.PathEscape(runID), "diffs")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("run diffs: %s", msg)
	}
	var listing struct {
		Diffs []struct {
			ID        string            `json:"id"`
			JobID     domaintypes.JobID `json:"job_id"`
			CreatedAt string            `json:"created_at"`
			Size      int               `json:"gzipped_size"`
		} `json:"diffs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return err
	}

	if !c.Download {
		for _, d := range listing.Diffs {
			job := strings.TrimSpace(d.JobID.String())
			if job == "" {
				job = "-"
			}
			_, _ = fmt.Fprintf(out, "%s job=%s size=%d created=%s\n", strings.TrimSpace(d.ID), job, d.Size, strings.TrimSpace(d.CreatedAt))
		}
		return nil
	}

	if len(listing.Diffs) == 0 {
		return errors.New("run diffs: no diffs available for this run")
	}
	// Newest first by API; take first.
	diffID := strings.TrimSpace(listing.Diffs[0].ID)

	// Download gzipped patch
	dlURL, err := url.JoinPath(c.BaseURL.String(), "v1", "diffs", url.PathEscape(diffID))
	if err != nil {
		return err
	}
	q := url.Values{"download": []string{"true"}}
	dlURL = dlURL + "?" + q.Encode()
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return err
	}
	resp2, err := c.Client.Do(req2)
	if err != nil {
		return err
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp2.Body)
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp2.Status
		}
		return fmt.Errorf("run diffs: %s", msg)
	}
	gzData, err := io.ReadAll(resp2.Body)
	if err != nil {
		return err
	}
	// gunzip
	patch, err := gunzipBytes(gzData)
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

// RepoDiffsCommand lists diffs for a specific repo execution within a run and
// optionally downloads the newest patch. This is the v1 repo-scoped version
// that replaces the legacy run-scoped DiffsCommand.
//
// Uses GET /v1/runs/{run_id}/repos/{repo_id}/diffs endpoint.
// Returns diffs filtered by repo_id via jobs.repo_id join.
type RepoDiffsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed)
	RepoID  string            // Repo ID (NanoID-backed)
	Output  io.Writer

	Download bool   // when true, download newest diff and print to stdout (gunzipped)
	SavePath string // optional path to save the gunzipped patch
}

// Run executes the command.
func (c RepoDiffsCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("run repo diffs: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("run repo diffs: base url required")
	}
	if c.RunID.IsZero() {
		return errors.New("run repo diffs: run id required")
	}
	if strings.TrimSpace(c.RepoID) == "" {
		return errors.New("run repo diffs: repo id required")
	}
	runID := c.RunID.String()
	repoID := c.RepoID
	out := c.Output
	if out == nil {
		out = io.Discard
	}

	// List diffs via repo-scoped endpoint
	listURL, err := url.JoinPath(c.BaseURL.String(), "v1", "runs", url.PathEscape(runID), "repos", url.PathEscape(repoID), "diffs")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("run repo diffs: %s", msg)
	}
	var listing struct {
		Diffs []struct {
			ID        string            `json:"id"`
			JobID     domaintypes.JobID `json:"job_id"`
			CreatedAt string            `json:"created_at"`
			Size      int               `json:"gzipped_size"`
		} `json:"diffs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return err
	}

	if !c.Download {
		for _, d := range listing.Diffs {
			job := strings.TrimSpace(d.JobID.String())
			if job == "" {
				job = "-"
			}
			_, _ = fmt.Fprintf(out, "%s job=%s size=%d created=%s\n", strings.TrimSpace(d.ID), job, d.Size, strings.TrimSpace(d.CreatedAt))
		}
		return nil
	}

	if len(listing.Diffs) == 0 {
		return errors.New("run repo diffs: no diffs available for this repo execution")
	}
	// Newest first by API; take first.
	diffID := strings.TrimSpace(listing.Diffs[0].ID)

	// Download gzipped patch
	dlURL, err := url.JoinPath(c.BaseURL.String(), "v1", "diffs", url.PathEscape(diffID))
	if err != nil {
		return err
	}
	q := url.Values{"download": []string{"true"}}
	dlURL = dlURL + "?" + q.Encode()
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return err
	}
	resp2, err := c.Client.Do(req2)
	if err != nil {
		return err
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp2.Body)
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp2.Status
		}
		return fmt.Errorf("run repo diffs: %s", msg)
	}
	gzData, err := io.ReadAll(resp2.Body)
	if err != nil {
		return err
	}
	// gunzip
	patch, err := gunzipBytes(gzData)
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

func gunzipBytes(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}
