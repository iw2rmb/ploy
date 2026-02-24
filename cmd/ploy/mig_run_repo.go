// mod_run_repo.go implements the `ploy mig run repo` subcommands for managing
// repos within a batch run.
//
// This file provides CLI routing for repo add/remove/restart/status operations
// that delegate to the control plane's /v1/runs/{id}/repos endpoints. Each
// subcommand parses its own flags and invokes the corresponding HTTP handler.
//
// Command structure:
//   - ploy mig run repo add --repo-url <url> --base-ref <ref> --target-ref <ref> <run-id>
//   - ploy mig run repo remove --repo-id <id> <run-id>
//   - ploy mig run repo restart --repo-id <id> [--base-ref <ref>] [--target-ref <ref>] <run-id>
//   - ploy mig run repo status <run-id>
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
	"strings"
	"text/tabwriter"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleMigRunRepo routes `mod run repo <action>` subcommands.
// Called when args[0] == "repo" in the mod run context.
func handleMigRunRepo(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printMigRunRepoUsage(stderr)
		return errors.New("mod run repo action required")
	}

	// Dispatch to the appropriate subcommand handler.
	switch args[0] {
	case "add":
		return handleMigRunRepoAdd(args[1:], stderr)
	case "remove":
		return handleMigRunRepoRemove(args[1:], stderr)
	case "restart":
		return handleMigRunRepoRestart(args[1:], stderr)
	case "status":
		return handleMigRunRepoStatus(args[1:], stderr)
	default:
		printMigRunRepoUsage(stderr)
		return fmt.Errorf("unknown mod run repo action %q", args[0])
	}
}

// printMigRunRepoUsage renders help for mod run repo subcommands.
func printMigRunRepoUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig run repo <action> [flags] <run-id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Actions:")
	_, _ = fmt.Fprintln(w, "  add       Add a repo to a batch run")
	_, _ = fmt.Fprintln(w, "  remove    Remove/cancel a repo from a batch run")
	_, _ = fmt.Fprintln(w, "  restart   Restart a repo within a batch run")
	_, _ = fmt.Fprintln(w, "  status    Show repos and their statuses within a batch run")
	_, _ = fmt.Fprintln(w, "")
	// Examples use neutral <repo-id> placeholder since repo IDs are NanoID(8) strings, not UUIDs.
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy mig run repo add --repo-url https://github.com/org/repo.git --base-ref main --target-ref feature <run-id>")
	_, _ = fmt.Fprintln(w, "  ploy mig run repo remove --repo-id <repo-id> <run-id>")
	_, _ = fmt.Fprintln(w, "  ploy mig run repo restart --repo-id <repo-id> <run-id>")
	_, _ = fmt.Fprintln(w, "  ploy mig run repo status <run-id>")
}

// handleMigRunRepoAdd implements `ploy mig run repo add <run-id> --repo-url <url> --base-ref <ref> --target-ref <ref>`.
// Adds a new repo entry to a batch run with status=Queued and immediately creates repo-scoped jobs.
func handleMigRunRepoAdd(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod run repo add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repoURL := fs.String("repo-url", "", "Git repository URL")
	baseRef := fs.String("base-ref", "", "Git base ref (branch or commit)")
	targetRef := fs.String("target-ref", "", "Git target ref (branch to create)")

	if err := fs.Parse(args); err != nil {
		printMigRunRepoUsage(stderr)
		return err
	}

	// Extract positional run-id.
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printMigRunRepoUsage(stderr)
		return errors.New("run-id required")
	}
	batchID := strings.TrimSpace(rest[0])

	// Validate required flags.
	trimmedRepoURL := strings.TrimSpace(*repoURL)
	if trimmedRepoURL == "" {
		printMigRunRepoUsage(stderr)
		return errors.New("--repo-url required")
	}
	if strings.TrimSpace(*baseRef) == "" {
		printMigRunRepoUsage(stderr)
		return errors.New("--base-ref required")
	}
	if strings.TrimSpace(*targetRef) == "" {
		printMigRunRepoUsage(stderr)
		return errors.New("--target-ref required")
	}
	if err := domaintypes.RepoURL(trimmedRepoURL).Validate(); err != nil {
		printMigRunRepoUsage(stderr)
		return fmt.Errorf("--repo-url: %w", err)
	}
	if err := domaintypes.GitRef(strings.TrimSpace(*baseRef)).Validate(); err != nil {
		printMigRunRepoUsage(stderr)
		return fmt.Errorf("--base-ref: %w", err)
	}
	if err := domaintypes.GitRef(strings.TrimSpace(*targetRef)).Validate(); err != nil {
		printMigRunRepoUsage(stderr)
		return fmt.Errorf("--target-ref: %w", err)
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Build and send the request to POST /v1/runs/{id}/repos.
	reqBody := runRepoAddRequest{
		RepoURL:   trimmedRepoURL,
		BaseRef:   strings.TrimSpace(*baseRef),
		TargetRef: strings.TrimSpace(*targetRef),
	}
	resp, err := doRunRepoAdd(ctx, base, httpClient, batchID, reqBody)
	if err != nil {
		return err
	}

	// v1: RepoID refers to mod_repos.id, the canonical repository identifier within the mod.
	_, _ = fmt.Fprintf(stderr, "Repo added: %s (repo_id: %s, status: %s)\n", domaintypes.NormalizeRepoURLSchemless(resp.RepoURL), resp.RepoID, resp.Status)
	return nil
}

// handleMigRunRepoRemove implements `ploy mig run repo remove <run-id> --repo-id <id>`.
// Cancels a repo within a run (Queued/Running → Cancelled) and cancels active jobs for the current attempt.
func handleMigRunRepoRemove(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod run repo remove", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	// Repo IDs are NanoID(8) strings; use neutral "identifier" wording.
	repoID := fs.String("repo-id", "", "Repo identifier to remove")

	if err := fs.Parse(args); err != nil {
		printMigRunRepoUsage(stderr)
		return err
	}

	// Extract positional run ID.
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printMigRunRepoUsage(stderr)
		return errors.New("run-id required")
	}
	batchID := strings.TrimSpace(rest[0])

	// Validate required flags.
	if strings.TrimSpace(*repoID) == "" {
		printMigRunRepoUsage(stderr)
		return errors.New("--repo-id required")
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Send POST /v1/runs/{id}/repos/{repo_id}/cancel.
	resp, err := doRunRepoRemove(ctx, base, httpClient, batchID, strings.TrimSpace(*repoID))
	if err != nil {
		return err
	}

	// v1: RepoID refers to mod_repos.id, the canonical repository identifier within the mod.
	_, _ = fmt.Fprintf(stderr, "Repo removed: %s (repo_id: %s, status: %s)\n", domaintypes.NormalizeRepoURLSchemless(resp.RepoURL), resp.RepoID, resp.Status)
	return nil
}

// handleMigRunRepoRestart implements `ploy mig run repo restart <run-id> --repo-id <id> [--base-ref <ref>] [--target-ref <ref>]`.
// Resets repo status to Queued, increments attempt, optionally updates refs.
func handleMigRunRepoRestart(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod run repo restart", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	// Repo IDs are NanoID(8) strings; use neutral "identifier" wording.
	repoID := fs.String("repo-id", "", "Repo identifier to restart")
	baseRef := fs.String("base-ref", "", "Optional new base ref")
	targetRef := fs.String("target-ref", "", "Optional new target ref")

	if err := fs.Parse(args); err != nil {
		printMigRunRepoUsage(stderr)
		return err
	}

	// Extract positional run ID.
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printMigRunRepoUsage(stderr)
		return errors.New("run-id required")
	}
	batchID := strings.TrimSpace(rest[0])

	// Validate required flags.
	if strings.TrimSpace(*repoID) == "" {
		printMigRunRepoUsage(stderr)
		return errors.New("--repo-id required")
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Build the optional request body for ref updates.
	reqBody := runRepoRestartRequest{}
	if br := strings.TrimSpace(*baseRef); br != "" {
		reqBody.BaseRef = &br
	}
	if tr := strings.TrimSpace(*targetRef); tr != "" {
		reqBody.TargetRef = &tr
	}

	// Send POST /v1/runs/{id}/repos/{repo_id}/restart.
	resp, err := doRunRepoRestart(ctx, base, httpClient, batchID, strings.TrimSpace(*repoID), reqBody)
	if err != nil {
		return err
	}

	// v1: RepoID refers to mod_repos.id, the canonical repository identifier within the mod.
	_, _ = fmt.Fprintf(stderr, "Repo restarted: %s (repo_id: %s, attempt: %d, status: %s)\n", domaintypes.NormalizeRepoURLSchemless(resp.RepoURL), resp.RepoID, resp.Attempt, resp.Status)
	return nil
}

// handleMigRunRepoStatus implements `ploy mig run repo status <run-id>`.
// Lists all repos within a batch with their status, attempt count, and timing.
func handleMigRunRepoStatus(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod run repo status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		printMigRunRepoUsage(stderr)
		return err
	}

	// Extract positional run ID.
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printMigRunRepoUsage(stderr)
		return errors.New("run-id required")
	}
	batchID := strings.TrimSpace(rest[0])

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Send GET /v1/runs/{id}/repos.
	repos, err := doRunRepoList(ctx, base, httpClient, batchID)
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		_, _ = fmt.Fprintln(stderr, "No repos found in this batch.")
		return nil
	}

	// Print table with repo details.
	// v1: Display repo_id (mod_repos.id) rather than a non-existent run_repos.id.
	tw := tabwriter.NewWriter(stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "REPO_ID\tREPO URL\tBASE REF\tTARGET REF\tATTEMPT\tSTATUS\tLAST ERROR")
	for _, r := range repos {
		lastErr := "-"
		if r.LastError != nil && *r.LastError != "" {
			// Truncate long error messages.
			errStr := *r.LastError
			if len(errStr) > 40 {
				errStr = errStr[:37] + "..."
			}
			lastErr = errStr
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			r.RepoID, domaintypes.NormalizeRepoURLSchemless(r.RepoURL), r.BaseRef, r.TargetRef, r.Attempt, r.Status, lastErr)
	}
	_ = tw.Flush()
	return nil
}

// -----------------------------------------------------------------------------
// HTTP client helpers for batch repo operations
// -----------------------------------------------------------------------------

// runRepoAddRequest is the request body for adding a repo to a batch.
type runRepoAddRequest struct {
	RepoURL   string `json:"repo_url"`
	BaseRef   string `json:"base_ref"`
	TargetRef string `json:"target_ref"`
}

// runRepoRestartRequest is the optional request body for restarting a repo.
type runRepoRestartRequest struct {
	BaseRef   *string `json:"base_ref,omitempty"`
	TargetRef *string `json:"target_ref,omitempty"`
}

// runRepoResponse mirrors the server's RunRepoResponse for CLI consumption.
// runRepoResponse represents a single repo within a batch for CLI responses.
// v1 model: run_repos uses composite PK (run_id, repo_id), not a standalone id field.
// RepoID refers to mod_repos.id (the repository identifier within a mod project).
type runRepoResponse struct {
	RunID      domaintypes.RunID     `json:"run_id"`
	RepoID     domaintypes.MigRepoID `json:"repo_id"` // mod_repos.id (NanoID, 8 chars)
	RepoURL    string                `json:"repo_url"`
	BaseRef    string                `json:"base_ref"`   // Snapshot from run_repos.repo_base_ref
	TargetRef  string                `json:"target_ref"` // Snapshot from run_repos.repo_target_ref
	Status     string                `json:"status"`
	Attempt    int32                 `json:"attempt"`
	LastError  *string               `json:"last_error,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	StartedAt  *time.Time            `json:"started_at,omitempty"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
}

// doRunRepoAdd sends POST /v1/runs/{id}/repos to add a repo to a batch.
func doRunRepoAdd(ctx context.Context, base *url.URL, client *http.Client, batchID string, req runRepoAddRequest) (runRepoResponse, error) {
	endpoint := base.JoinPath("v1", "runs", batchID, "repos")

	body, err := json.Marshal(req)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return runRepoResponse{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var result runRepoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return runRepoResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// doRunRepoRemove sends POST /v1/runs/{id}/repos/{repo_id}/cancel to cancel a repo execution.
func doRunRepoRemove(ctx context.Context, base *url.URL, client *http.Client, batchID, repoID string) (runRepoResponse, error) {
	endpoint := base.JoinPath("v1", "runs", batchID, "repos", repoID, "cancel")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return runRepoResponse{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var result runRepoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return runRepoResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// doRunRepoRestart sends POST /v1/runs/{id}/repos/{repo_id}/restart to restart a repo.
func doRunRepoRestart(ctx context.Context, base *url.URL, client *http.Client, batchID, repoID string, req runRepoRestartRequest) (runRepoResponse, error) {
	endpoint := base.JoinPath("v1", "runs", batchID, "repos", repoID, "restart")

	body, err := json.Marshal(req)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return runRepoResponse{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var result runRepoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return runRepoResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// doRunRepoList sends GET /v1/runs/{id}/repos to list repos within a batch.
func doRunRepoList(ctx context.Context, base *url.URL, client *http.Client, batchID string) ([]runRepoResponse, error) {
	endpoint := base.JoinPath("v1", "runs", batchID, "repos")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	// Response is {"repos": [...]}
	var result struct {
		Repos []runRepoResponse `json:"repos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result.Repos, nil
}
