package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	runcmd "github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func handleRerun(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printRerunUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("rerun", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jobID := fs.String("job", "", "Source job ID to rerun from")
	alterPath := fs.String("alter", "", "Path to alter overlay YAML/JSON")
	follow := fs.Bool("follow", false, "Follow run until completion")
	if err := parseFlagSet(fs, args, func() { printRerunUsage(stderr) }); err != nil {
		return err
	}
	if strings.TrimSpace(*jobID) == "" {
		printRerunUsage(stderr)
		return errors.New("--job is required")
	}
	if strings.TrimSpace(*alterPath) == "" {
		printRerunUsage(stderr)
		return errors.New("--alter is required")
	}

	alterRaw, err := loadRerunAlterFile(*alterPath)
	if err != nil {
		return err
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	result, err := doRerunRequest(ctx, base, httpClient, strings.TrimSpace(*jobID), alterRaw)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Rerun queued: root_job_id=%s run_id=%s repo_id=%s attempt=%d source_job_id=%s\n",
		result.RootJobID,
		result.RunID,
		result.RepoID,
		result.Attempt,
		result.CopiedFromJobID,
	)

	if !*follow {
		return nil
	}

	_, err = runcmd.FollowRunCommand{
		Client:     httpClient,
		BaseURL:    base,
		RunID:      result.RunID,
		Output:     stderr,
		MaxRetries: 5,
	}.Run(ctx)
	return err
}

type rerunResult struct {
	RunID           domaintypes.RunID  `json:"run_id"`
	RepoID          domaintypes.RepoID `json:"repo_id"`
	Attempt         int32              `json:"attempt"`
	RootJobID       domaintypes.JobID  `json:"root_job_id"`
	CopiedFromJobID domaintypes.JobID  `json:"copied_from_job_id"`
}

func doRerunRequest(ctx context.Context, baseURL *url.URL, client *http.Client, sourceJobID string, alter map[string]any) (rerunResult, error) {
	endpoint := baseURL.JoinPath("v1", "jobs", sourceJobID, "rerun")
	payload := map[string]any{"alter": alter}
	body, err := json.Marshal(payload)
	if err != nil {
		return rerunResult{}, fmt.Errorf("rerun: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return rerunResult{}, fmt.Errorf("rerun: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return rerunResult{}, fmt.Errorf("rerun: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode != http.StatusCreated {
		return rerunResult{}, httpx.WrapError("rerun", resp.Status, resp.Body)
	}

	var result rerunResult
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return rerunResult{}, fmt.Errorf("rerun: decode response: %w", err)
	}
	if result.RootJobID.IsZero() || result.RunID.IsZero() {
		return rerunResult{}, fmt.Errorf("rerun: invalid response payload")
	}
	return result, nil
}

func loadRerunAlterFile(path string) (map[string]any, error) {
	resolved, err := resolvePath(path)
	if err != nil {
		return nil, fmt.Errorf("rerun: resolve alter path: %w", err)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("rerun: read alter file %s: %w", resolved, err)
	}
	obj, err := parseSpecInputToMap(data, filepath.Dir(resolved))
	if err != nil {
		return nil, fmt.Errorf("rerun: parse alter file %s (not valid JSON or YAML): %w", resolved, err)
	}
	return obj, nil
}

func printRerunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy rerun --job <job-id> --alter <path.yaml|json> [--follow]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --job <job-id>       Source terminal job ID (heal/re_gate)")
	_, _ = fmt.Fprintln(w, "  --alter <path>       Alter overlay file (YAML/JSON) with image/envs/in")
	_, _ = fmt.Fprintln(w, "  --follow             Follow run until completion")
}
