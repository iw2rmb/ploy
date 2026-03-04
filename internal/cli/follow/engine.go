// Package follow implements the job graph follow mode for CLI.
// It displays a summarized per-repo job graph that refreshes until run completion.
package follow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// Config holds follow engine configuration.
type Config struct {
	MaxRetries int
	Output     io.Writer
}

// RepoEntry represents a repo in the run.
type RepoEntry struct {
	RunID      domaintypes.RunID         `json:"run_id"`
	RepoID     domaintypes.MigRepoID     `json:"repo_id"`
	RepoURL    string                    `json:"repo_url"`
	BaseRef    string                    `json:"base_ref"`
	TargetRef  string                    `json:"target_ref"`
	Status     domaintypes.RunRepoStatus `json:"status"`
	Attempt    int32                     `json:"attempt"`
	LastError  *string                   `json:"last_error,omitempty"`
	CreatedAt  time.Time                 `json:"created_at"`
	StartedAt  *time.Time                `json:"started_at,omitempty"`
	FinishedAt *time.Time                `json:"finished_at,omitempty"`
}

// Engine manages the follow mode rendering loop.
type Engine struct {
	client    *http.Client
	streamCli stream.Client
	baseURL   *url.URL
	runID     domaintypes.RunID
	config    Config

	mu           sync.Mutex
	repoOrder    []domaintypes.MigRepoID          // Order for stable rendering
	repoURLs     map[domaintypes.MigRepoID]string // RepoID -> RepoURL
	repoJobs     map[domaintypes.MigRepoID][]runs.RepoJobEntry
	repoErrors   map[domaintypes.MigRepoID]*string // RepoID -> LastError
	spinnerFrame int                               // Current spinner animation frame

	renderedLines int  // number of lines rendered in last frame
	renderStarted bool // whether we've emitted initial frame controls
}

type refreshKind int

const (
	refreshJobs refreshKind = iota
	refreshReposAndJobs
)

const (
	ansiRed   = "\x1b[31m"
	ansiReset = "\x1b[0m"
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
		repoOrder:  make([]domaintypes.MigRepoID, 0),
		repoURLs:   make(map[domaintypes.MigRepoID]string),
		repoJobs:   make(map[domaintypes.MigRepoID][]runs.RepoJobEntry),
		repoErrors: make(map[domaintypes.MigRepoID]*string),
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
	repoIDs := make([]domaintypes.MigRepoID, len(e.repoOrder))
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

	frame := runs.FollowFrame{
		TopLines: []string{fmt.Sprintf("  Repos: %d", len(e.repoOrder))},
		Repos:    make([]runs.FollowRepoFrame, 0, len(e.repoOrder)),
	}

	for i, repoID := range e.repoOrder {
		repoURL := e.repoURLs[repoID]
		jobs := e.repoJobs[repoID]
		repoErr := runs.FormatErrorOneLiner(e.repoErrors[repoID])
		repoErrRendered := false

		repoFrame := runs.FollowRepoFrame{
			HeaderLine: fmt.Sprintf("  Repo %d/%d: %s", i+1, len(e.repoOrder), repoURL),
			Columns:    []string{"", "Step", "Job ID", "Node", "Image", "Duration"},
			Rows:       make([]runs.FollowStepRow, 0, len(jobs)),
		}

		for _, job := range jobs {
			nodeID := runs.FormatNodeID(job.NodeID)
			image := strings.TrimSpace(job.JobImage)
			if image == "" {
				image = "-"
			}

			glyph := runs.StatusGlyph(string(job.Status), e.spinnerFrame)
			duration := runs.FormatDurationMsOrElapsed(job.DurationMs, job.StartedAt, job.FinishedAt, time.Now())

			row := runs.FollowStepRow{
				Cells: []string{
					glyph,
					job.JobType.String(),
					job.JobID.String(),
					nodeID,
					image,
					duration,
				},
			}
			if !repoErrRendered && repoErr != "" && isFailedStatus(string(job.Status)) {
				row.ExitOneLiner = ansiRed + "└ " + repoErr + ansiReset
				repoErrRendered = true
			}
			repoFrame.Rows = append(repoFrame.Rows, row)
		}
		if repoErr != "" && !repoErrRendered {
			if len(repoFrame.Rows) > 0 {
				repoFrame.Rows[len(repoFrame.Rows)-1].ExitOneLiner = ansiRed + "└ " + repoErr + ansiReset
			} else {
				repoFrame.EmptyLine = ansiRed + "└ " + repoErr + ansiReset
			}
		}
		frame.Repos = append(frame.Repos, repoFrame)
	}

	rendered, lines := runs.RenderFollowFrameText(frame)

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
	fmt.Fprint(out, rendered)
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

func isFailedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "fail", "failed":
		return true
	default:
		return false
	}
}

func isTerminalState(s modsapi.RunState) bool {
	switch s {
	case modsapi.RunStateSucceeded, modsapi.RunStateFailed, modsapi.RunStateCancelled:
		return true
	}
	return false
}
