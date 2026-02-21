// Package follow implements the job graph follow mode for CLI.
// It displays a summarized per-repo job graph that refreshes until run completion.
package follow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
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
	RunID      domaintypes.RunID     `json:"run_id"`
	RepoID     domaintypes.ModRepoID `json:"repo_id"`
	RepoURL    string                `json:"repo_url"`
	BaseRef    string                `json:"base_ref"`
	TargetRef  string                `json:"target_ref"`
	Status     store.RunRepoStatus   `json:"status"`
	Attempt    int32                 `json:"attempt"`
	LastError  *string               `json:"last_error,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	StartedAt  *time.Time            `json:"started_at,omitempty"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
}

// spinnerFrames defines the animation frames for the running spinner.
var spinnerFrames = []string{"⣾ ", "⣽ ", "⣻ ", "⢿ ", "⡿ ", "⣟ ", "⣯ ", "⣷ "}

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
	repoErrors   map[domaintypes.ModRepoID]*string // RepoID -> LastError
	spinnerFrame int                               // Current spinner animation frame

	renderedLines int  // number of lines rendered in last frame
	renderStarted bool // whether we've emitted initial frame controls
}

type refreshKind int

const (
	refreshJobs refreshKind = iota
	refreshReposAndJobs
)

// NewEngine creates a follow engine.
func NewEngine(httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID, cfg Config) *Engine {
	streamClient := stream.Client{
		HTTPClient: httpClient,
		MaxRetries: cfg.MaxRetries,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})),
	}
	return &Engine{
		client:     httpClient,
		streamCli:  streamClient,
		baseURL:    baseURL,
		runID:      runID,
		config:     cfg,
		repoOrder:  make([]domaintypes.ModRepoID, 0),
		repoURLs:   make(map[domaintypes.ModRepoID]string),
		repoJobs:   make(map[domaintypes.ModRepoID][]runs.RepoJobEntry),
		repoErrors: make(map[domaintypes.ModRepoID]*string),
	}
}

// Run executes the follow loop until run reaches terminal state.
func (e *Engine) Run(ctx context.Context) (modsapi.RunState, error) {
	defer e.showCursor()

	// 1. Initial fetch of repos and jobs.
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
	finalMu := &sync.Mutex{}

	refreshCh := make(chan refreshKind, 1)
	streamErrCh := make(chan error, 1)

	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "run":
			var summary modsapi.RunSummary
			if err := json.Unmarshal(evt.Data, &summary); err == nil {
				if isTerminalState(summary.State) {
					finalMu.Lock()
					finalState = summary.State
					finalMu.Unlock()
					select {
					case refreshCh <- refreshReposAndJobs:
					default:
					}
					return stream.ErrDone
				}
			}
			// Refresh repos and jobs on run event (new repos may have been added).
			select {
			case refreshCh <- refreshReposAndJobs:
			default:
			}
		case "stage":
			// Refresh on stage changes (job status updates).
			select {
			case refreshCh <- refreshJobs:
			default:
			}
		}
		return nil
	}

	go func() {
		streamErrCh <- e.streamCli.Stream(ctx, endpoint, handler)
	}()

	// Periodic ticks:
	// - renderTicker drives the spinner and running duration updates
	// - refreshTicker keeps job status snapshots fresh even if SSE only emits logs
	const renderInterval = 200 * time.Millisecond
	const refreshInterval = 1 * time.Second
	renderTicker := time.NewTicker(renderInterval)
	refreshTicker := time.NewTicker(refreshInterval)
	defer renderTicker.Stop()
	defer refreshTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case err := <-streamErrCh:
			// Final refresh + render for a stable terminal snapshot.
			_ = e.refreshRepos(ctx)
			_ = e.refreshAllJobs(ctx)
			e.render()

			if err != nil {
				return "", err
			}
			finalMu.Lock()
			out := finalState
			finalMu.Unlock()
			return out, nil
		case kind := <-refreshCh:
			if kind == refreshReposAndJobs {
				_ = e.refreshRepos(ctx)
			}
			_ = e.refreshAllJobs(ctx)
			e.render()
		case <-refreshTicker.C:
			_ = e.refreshAllJobs(ctx)
			e.render()
		case <-renderTicker.C:
			e.render()
		}
	}
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
		repoURL := strings.TrimSpace(repo.RepoURL)
		if repoURL == "" {
			repoURL = repo.RepoID.String()
		} else {
			repoURL = domaintypes.NormalizeRepoURLSchemless(repoURL)
		}
		e.repoURLs[repo.RepoID] = repoURL
		e.repoErrors[repo.RepoID] = repo.LastError
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

	// Build the full frame into a buffer so we can count lines and redraw in-place.
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 8, 2, ' ', 0)

	_, _ = fmt.Fprintln(tw, "Repo\tIndex\tModType\tJob ID\tNodeID\tImage\tSpin\tDuration\tStatus")

	for _, repoID := range e.repoOrder {
		repoURL := e.repoURLs[repoID]
		jobs := e.repoJobs[repoID]

		if len(jobs) == 0 {
			fmt.Fprintf(tw, "%s\t\t\t\t\t\t\t\t\n", repoURL)
			continue
		}

		for i, job := range jobs {
			repoCol := ""
			if i == 0 {
				repoCol = repoURL
			}

			nodeID := "-"
			if job.NodeID != nil && !job.NodeID.IsZero() {
				nodeID = job.NodeID.String()
			}
			image := strings.TrimSpace(job.ModImage)
			if image == "" {
				image = "-"
			}

			glyph := statusGlyph(string(job.Status), e.spinnerFrame)
			duration := formatDuration(job)
			status := strings.ToLower(string(job.Status))

			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				repoCol,
				formatStepIndex(job.StepIndex),
				job.ModType,
				job.JobID.String(),
				nodeID,
				image,
				glyph,
				duration,
				status,
			)
		}
		// Display last_error if present
		if lastErr := e.repoErrors[repoID]; lastErr != nil && *lastErr != "" {
			fmt.Fprintf(tw, "\t\t\t\t\t\t\t\t\n")
			for _, line := range strings.Split(*lastErr, "\n") {
				fmt.Fprintf(tw, "\t✗ %s\t\t\t\t\t\t\t\n", line)
			}
		}
		_, _ = fmt.Fprintln(tw)
	}

	_ = tw.Flush()
	frame := buf.String()
	lines := strings.Count(frame, "\n")

	// In-place redraw: move cursor up over the previously rendered frame and clear.
	// This keeps output stable (like package managers / docker build) instead of
	// jumping to the top of the terminal each frame.
	if !e.renderStarted {
		// Hide cursor while animating to reduce flicker.
		fmt.Fprint(out, "\x1b[?25l")
		e.renderStarted = true
	} else if e.renderedLines > 0 {
		fmt.Fprintf(out, "\x1b[%dA", e.renderedLines)
		fmt.Fprint(out, "\x1b[J")
	}
	fmt.Fprint(out, frame)
	e.renderedLines = lines

	// Advance spinner frame for next render
	e.spinnerFrame++
}

func (e *Engine) showCursor() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.config.Output == nil {
		return
	}
	if e.renderStarted {
		fmt.Fprint(e.config.Output, "\x1b[?25h")
	}
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
	case "created":
		return "·"
	case "queued":
		return "·"
	default:
		return " "
	}
}

func formatStepIndex(v domaintypes.StepIndex) string {
	return strconv.FormatFloat(float64(v), 'f', -1, 64)
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
