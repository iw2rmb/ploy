// Package follow implements the job graph follow mode for CLI.
// It displays a summarized per-repo job graph that refreshes until run completion.
package follow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/store"
)

// Config holds follow engine configuration.
type Config struct {
	MaxRetries int
	Output     io.Writer
}

// RepoEntry represents a repo in the run.
type RepoEntry struct {
	RepoID    domaintypes.ModRepoID `json:"repo_id"`
	RepoURL   string                `json:"repo_url"`
	BaseRef   string                `json:"base_ref"`
	TargetRef string                `json:"target_ref"`
	Status    store.RunRepoStatus   `json:"status"`
	Attempt   int32                 `json:"attempt"`
}

// spinnerFrames defines the animation frames for the running spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Engine manages the follow mode rendering loop.
type Engine struct {
	client    *http.Client
	streamCli stream.Client
	baseURL   *url.URL
	runID     domaintypes.RunID
	config    Config

	mu           sync.Mutex
	repoOrder    []domaintypes.ModRepoID          // Order for stable rendering
	repoURLs     map[domaintypes.ModRepoID]string // RepoID -> RepoURL
	repoJobs     map[domaintypes.ModRepoID][]runs.RepoJobEntry
	spinnerFrame int // Current spinner animation frame
}

// NewEngine creates a follow engine.
func NewEngine(httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID, cfg Config) *Engine {
	streamClient := stream.Client{
		HTTPClient: httpClient,
		MaxRetries: cfg.MaxRetries,
	}
	return &Engine{
		client:    httpClient,
		streamCli: streamClient,
		baseURL:   baseURL,
		runID:     runID,
		config:    cfg,
		repoOrder: make([]domaintypes.ModRepoID, 0),
		repoURLs:  make(map[domaintypes.ModRepoID]string),
		repoJobs:  make(map[domaintypes.ModRepoID][]runs.RepoJobEntry),
	}
}

// Run executes the follow loop until run reaches terminal state.
func (e *Engine) Run(ctx context.Context) (modsapi.RunState, error) {
	// 1. Initial fetch of repos and jobs
	if err := e.refreshRepos(ctx); err != nil {
		return "", err
	}
	if err := e.refreshAllJobs(ctx); err != nil {
		return "", err
	}
	e.render()

	// 2. Subscribe to SSE and refresh on events
	endpoint, err := url.JoinPath(e.baseURL.String(), "v1", "runs", e.runID.String(), "logs")
	if err != nil {
		return "", fmt.Errorf("follow: build endpoint: %w", err)
	}

	var finalState modsapi.RunState
	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "run":
			var summary modsapi.RunSummary
			if err := json.Unmarshal(evt.Data, &summary); err == nil {
				if isTerminalState(summary.State) {
					finalState = summary.State
					// Final refresh and render
					_ = e.refreshRepos(ctx)
					_ = e.refreshAllJobs(ctx)
					e.render()
					return stream.ErrDone
				}
			}
			// Refresh repos and jobs on run event (new repos may have been added)
			_ = e.refreshRepos(ctx)
			_ = e.refreshAllJobs(ctx)
			e.render()
		case "stage":
			// Refresh on stage changes (job status updates)
			_ = e.refreshAllJobs(ctx)
			e.render()
			// Ignore "log" events - we don't display logs in follow mode
		}
		return nil
	}

	if err := e.streamCli.Stream(ctx, endpoint, handler); err != nil {
		return "", err
	}

	return finalState, nil
}

// refreshRepos fetches the current repos in the run.
func (e *Engine) refreshRepos(ctx context.Context) error {
	endpoint := e.baseURL.JoinPath("v1", "runs", e.runID.String(), "repos")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("follow: build repos request: %w", err)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("follow: fetch repos failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return httpx.WrapError("follow: fetch repos", resp.Status, resp.Body)
	}

	var result struct {
		Repos []RepoEntry `json:"repos"`
	}
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return fmt.Errorf("follow: decode repos: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Track repo order for stable rendering (new repos append to list)
	for _, repo := range result.Repos {
		if _, exists := e.repoURLs[repo.RepoID]; !exists {
			// New repo discovered - add to order list
			e.repoOrder = append(e.repoOrder, repo.RepoID)
		}
		// Update or set repo URL
		url := repo.RepoURL
		if url == "" {
			url = repo.RepoID.String()
		}
		e.repoURLs[repo.RepoID] = url
	}

	return nil
}

// refreshAllJobs fetches jobs for all repos.
func (e *Engine) refreshAllJobs(ctx context.Context) error {
	e.mu.Lock()
	repoIDs := make([]domaintypes.ModRepoID, len(e.repoOrder))
	copy(repoIDs, e.repoOrder)
	e.mu.Unlock()

	for _, repoID := range repoIDs {
		result, err := runs.ListRepoJobsCommand{
			Client:  e.client,
			BaseURL: e.baseURL,
			RunID:   e.runID,
			RepoID:  repoID,
		}.Run(ctx)
		if err != nil {
			continue // Best effort - don't fail on individual repo errors
		}

		e.mu.Lock()
		e.repoJobs[repoID] = result.Jobs
		e.mu.Unlock()
	}

	return nil
}

// render outputs the job graph to the configured writer.
func (e *Engine) render() {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := e.config.Output
	if out == nil {
		return
	}

	// Clear screen and move cursor to top (ANSI escape codes)
	fmt.Fprint(out, "\033[2J\033[H")

	for _, repoID := range e.repoOrder {
		repoURL := e.repoURLs[repoID]
		jobs := e.repoJobs[repoID]

		fmt.Fprintf(out, "%s\n", repoURL)

		for _, job := range jobs {
			displayName := job.DisplayName
			if displayName == "" {
				displayName = job.Name
			}

			glyph := statusGlyph(string(job.Status), e.spinnerFrame)
			duration := formatDuration(job)
			status := strings.ToLower(string(job.Status))

			fmt.Fprintf(out, "  %v %s %s %s %s %s %s\n",
				job.StepIndex.Float64(), job.ModType, job.JobID.String(),
				displayName, glyph, duration, status)
		}
		fmt.Fprintln(out)
	}

	// Advance spinner frame for next render
	e.spinnerFrame++
}

// statusGlyph returns the display glyph for a job status.
// For running jobs, spinnerFrame controls the animation frame.
func statusGlyph(status string, spinnerFrame int) string {
	switch strings.ToLower(status) {
	case "running":
		// Animated spinner for running jobs
		return spinnerFrames[spinnerFrame%len(spinnerFrames)]
	case "success":
		return "✓"
	case "fail":
		return "✗"
	case "cancelled":
		return "○"
	case "queued":
		return "·"
	default:
		return " "
	}
}

func formatDuration(job runs.RepoJobEntry) string {
	if job.DurationMs > 0 {
		return fmt.Sprintf("%dms", job.DurationMs)
	}
	// Fallback: compute from timestamps if duration_ms not set
	if job.FinishedAt != nil && job.StartedAt != nil {
		// Terminal job: use finished_at - started_at
		d := job.FinishedAt.Sub(*job.StartedAt)
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if job.StartedAt != nil {
		// Running job: use time since started
		d := time.Since(*job.StartedAt)
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return "-"
}

func isTerminalState(s modsapi.RunState) bool {
	switch s {
	case modsapi.RunStateSucceeded, modsapi.RunStateFailed, modsapi.RunStateCancelled:
		return true
	}
	return false
}
