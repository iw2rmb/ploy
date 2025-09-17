package git

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CommandRunner executes system commands; allows substitution in tests.
type CommandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) error
}

// ExecRunner executes commands via os/exec.
type ExecRunner struct{}

// Run executes the command in the provided directory, capturing stderr for context.
func (ExecRunner) Run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, stderrStr)
		}
		return err
	}
	return nil
}

// EventType represents the type of event emitted by git operations.
type EventType string

const (
	EventStarted   EventType = "started"
	EventProgress  EventType = "progress"
	EventCompleted EventType = "completed"
	EventFailed    EventType = "failed"
)

// Event captures the lifecycle of a git operation.
type Event struct {
	Type      EventType
	Operation string
	Message   string
	Err       error
}

// EventSink receives emitted git operation events.
type EventSink interface {
	Publish(Event)
}

// ServiceConfig configures a new Service instance.
type ServiceConfig struct {
	Runner    CommandRunner
	EventSink EventSink
}

// Service provides Git functionality and emits structured events.
type Service struct {
	runner CommandRunner
	sink   EventSink
}

// NewService constructs a Service with the provided configuration.
func NewService(cfg ServiceConfig) *Service {
	runner := cfg.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Service{runner: runner, sink: cfg.EventSink}
}

// NewGitOperations preserves the previous constructor signature for callers that
// only need a service with default configuration.
func NewGitOperations(workDir string) *Service {
	_ = workDir // deprecated; service no longer requires a working directory hint
	return NewService(ServiceConfig{})
}

// SetEventSink updates the sink used for emitting events; primarily for wiring observers post-construction.
func (s *Service) SetEventSink(sink EventSink) {
	s.sink = sink
}

// Operation represents a long-running git action with observable events.
type Operation struct {
	name   string
	events chan Event
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	err    error
}

func newOperation(name string) *Operation {
	return &Operation{
		name:   name,
		events: make(chan Event, 6),
		done:   make(chan struct{}),
	}
}

// Events returns a channel for streaming operation events until completion.
func (o *Operation) Events() <-chan Event { return o.events }

// Wait blocks until the operation completes and returns its terminal error, if any.
func (o *Operation) Wait() error {
	<-o.done
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.err
}

// Err is an alias for Wait to interoperate with legacy call sites.
func (o *Operation) Err() error { return o.Wait() }

func (o *Operation) emit(event Event) {
	o.events <- event
}

func (o *Operation) finalize(event Event) {
	o.mu.Lock()
	o.err = event.Err
	o.mu.Unlock()
	o.events <- event
	o.once.Do(func() {
		close(o.events)
		close(o.done)
	})
}

func (s *Service) emit(op *Operation, event Event) {
	op.emit(event)
	if s.sink != nil {
		s.sink.Publish(event)
	}
}

func (s *Service) finalize(op *Operation, event Event) {
	op.finalize(event)
	if s.sink != nil {
		s.sink.Publish(event)
	}
}

// PushRequest encapsulates the inputs for a push operation.
type PushRequest struct {
	RepoPath  string
	RemoteURL string
	Branch    string
}

// DiffCapture captures code changes made during git operations.
type DiffCapture struct {
	File         string    `json:"file"`
	Type         string    `json:"type"`
	Before       string    `json:"before,omitempty"`
	After        string    `json:"after,omitempty"`
	UnifiedDiff  string    `json:"unified_diff"`
	LinesAdded   int       `json:"lines_added"`
	LinesRemoved int       `json:"lines_removed"`
	Timestamp    time.Time `json:"timestamp"`
}

// CreateBranchAndCheckout creates a new branch and switches to it (or just checks out if exists)
func (g *Service) CreateBranchAndCheckout(ctx context.Context, repoPath, branchName string) error {
	// Try to create new branch
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", branchName)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		// If branch exists, just checkout
		co := exec.CommandContext(ctx, "git", "checkout", branchName)
		co.Dir = repoPath
		if err2 := co.Run(); err2 != nil {
			return fmt.Errorf("failed to checkout branch %s: %w", branchName, err2)
		}
	}
	return nil
}

// PushBranchAsync executes a git push asynchronously and emits lifecycle events.
func (s *Service) PushBranchAsync(ctx context.Context, req PushRequest) *Operation {
	op := newOperation("push")
	go func() {
		s.emit(op, Event{
			Type:      EventStarted,
			Operation: op.name,
			Message:   fmt.Sprintf("pushing %s to %s", req.Branch, req.RemoteURL),
		})

		remoteURL := s.authenticatedRemoteURL(req.RemoteURL)
		_ = s.runner.Run(ctx, req.RepoPath, "git", "remote", "remove", "origin")
		if err := s.runner.Run(ctx, req.RepoPath, "git", "remote", "add", "origin", remoteURL); err != nil {
			wrapped := fmt.Errorf("failed to set remote origin: %w", err)
			s.finalize(op, Event{
				Type:      EventFailed,
				Operation: op.name,
				Message:   wrapped.Error(),
				Err:       wrapped,
			})
			return
		}

		s.emit(op, Event{
			Type:      EventProgress,
			Operation: op.name,
			Message:   "remote origin configured",
		})

		if err := s.runner.Run(ctx, req.RepoPath, "git", "push", "-u", "origin", req.Branch); err != nil {
			wrapped := fmt.Errorf("git push failed: %w", err)
			s.finalize(op, Event{
				Type:      EventFailed,
				Operation: op.name,
				Message:   wrapped.Error(),
				Err:       wrapped,
			})
			return
		}

		s.finalize(op, Event{
			Type:      EventCompleted,
			Operation: op.name,
			Message:   "branch pushed",
		})
	}()
	return op
}

// PushBranch is the synchronous wrapper maintained for compatibility with existing call sites.
func (s *Service) PushBranch(ctx context.Context, repoPath, remoteURL, branchName string) error {
	op := s.PushBranchAsync(ctx, PushRequest{RepoPath: repoPath, RemoteURL: remoteURL, Branch: branchName})
	return op.Wait()
}

// authenticatedRemoteURL injects credentials into the remote URL when possible.
// For GitLab, using username "oauth2" with the token as password works for PATs
// and project/group access tokens. Only applies to http/https URLs.
func (g *Service) authenticatedRemoteURL(remote string) string {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return remote
	}
	u, err := url.Parse(remote)
	if err != nil {
		return remote
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return remote
	}
	// Avoid overwriting if userinfo already present
	if u.User != nil {
		return remote
	}
	// url.UserPassword will handle necessary escaping
	u.User = url.UserPassword("oauth2", token)
	return u.String()
}

// checkGitAvailable verifies that git is installed and accessible
func (g *Service) checkGitAvailable() error {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git is not installed or not accessible: %w - Please ensure git is installed on the system", err)
	}
	return nil
}

// CloneRepository clones a Git repository to the specified path
func (g *Service) CloneRepository(ctx context.Context, repoURL, branch, targetPath string) error {
	// Check if git is available
	if err := g.checkGitAvailable(); err != nil {
		return fmt.Errorf("git dependency check failed: %w", err)
	}

	// Create parent directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Normalize branch ref (support refs/heads/*)
	normalized := strings.TrimPrefix(branch, "refs/heads/")
	// Build clone command
	args := []string{"clone", "--depth", "1", "--single-branch"}
	if normalized != "" && normalized != "main" && normalized != "master" {
		args = append(args, "--branch", normalized)
	}
	args = append(args, repoURL, targetPath)

	// Execute git clone
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %v - %s", err, stderr.String())
	}

	// If branch wasn't specified during clone, checkout the branch
	if normalized != "" && normalized != "main" && normalized != "master" {
		checkoutCmd := exec.CommandContext(ctx, "git", "checkout", normalized)
		checkoutCmd.Dir = targetPath
		if err := checkoutCmd.Run(); err != nil {
			// Try to fetch the branch first
			fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", normalized)
			fetchCmd.Dir = targetPath
			_ = fetchCmd.Run() // Ignore fetch errors

			// Try checkout again
			if err := checkoutCmd.Run(); err != nil {
				return fmt.Errorf("failed to checkout branch %s: %w", normalized, err)
			}
		}
	}

	// Ensure sparse checkout is disabled and working tree is fully populated (best-effort)
	{
		cfg := exec.CommandContext(ctx, "git", "config", "core.sparseCheckout", "false")
		cfg.Dir = targetPath
		_ = cfg.Run()
		disable := exec.CommandContext(ctx, "git", "sparse-checkout", "disable")
		disable.Dir = targetPath
		_ = disable.Run()
		reset := exec.CommandContext(ctx, "git", "reset", "--hard", "HEAD")
		reset.Dir = targetPath
		_ = reset.Run()
	}

	// Post-clone sanity: ensure repository is not empty
	if fi, err := os.Stat(filepath.Join(targetPath, ".git")); err != nil || !fi.IsDir() {
		return fmt.Errorf("git clone produced no .git directory at %s", targetPath)
	}
	if entries, err := os.ReadDir(targetPath); err == nil {
		nonMeta := 0
		for _, e := range entries {
			if e.Name() == ".git" {
				continue
			}
			nonMeta++
		}
		if nonMeta == 0 {
			return fmt.Errorf("git clone empty working tree at %s (no files besides .git)", targetPath)
		}
	}

	return nil
}

// GetDiff captures the diff of changes in the repository
func (g *Service) GetDiff(ctx context.Context, repoPath string) ([]DiffCapture, error) {
	// Get list of modified files
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = repoPath

	statusOutput, err := statusCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git status: %w", err)
	}

	var diffs []DiffCapture
	lines := strings.Split(string(statusOutput), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Parse status line (e.g., "M  file.txt" or "A  newfile.txt")
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		file := parts[1]

		diff := DiffCapture{
			File:      file,
			Timestamp: time.Now(),
		}

		switch status {
		case "M", "MM": // Modified
			diff.Type = "modified"
			// Get the actual diff
			diffCmd := exec.CommandContext(ctx, "git", "diff", file)
			diffCmd.Dir = repoPath
			diffOutput, _ := diffCmd.Output()
			diff.UnifiedDiff = string(diffOutput)

		case "A", "AM": // Added
			diff.Type = "added"
			// Get file contents as the diff
			content, _ := os.ReadFile(filepath.Join(repoPath, file))
			diff.After = string(content)
			diff.UnifiedDiff = fmt.Sprintf("+++ %s\n%s", file, string(content))

		case "D": // Deleted
			diff.Type = "deleted"
			// Get the diff from HEAD
			diffCmd := exec.CommandContext(ctx, "git", "diff", "HEAD", file)
			diffCmd.Dir = repoPath
			diffOutput, _ := diffCmd.Output()
			diff.UnifiedDiff = string(diffOutput)

		case "??": // Untracked
			diff.Type = "added"
			content, _ := os.ReadFile(filepath.Join(repoPath, file))
			diff.After = string(content)
			diff.UnifiedDiff = fmt.Sprintf("+++ %s (new file)\n%s", file, string(content))
		}

		diffs = append(diffs, diff)
	}

	// Also get staged changes
	stagedCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--name-status")
	stagedCmd.Dir = repoPath
	stagedOutput, _ := stagedCmd.Output()

	if len(stagedOutput) > 0 {
		stagedLines := strings.Split(string(stagedOutput), "\n")
		for _, line := range stagedLines {
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Get the unified diff for staged files
				diffCmd := exec.CommandContext(ctx, "git", "diff", "--cached", parts[1])
				diffCmd.Dir = repoPath
				diffOutput, _ := diffCmd.Output()

				diffs = append(diffs, DiffCapture{
					File:        parts[1],
					Type:        "modified",
					UnifiedDiff: string(diffOutput),
					Timestamp:   time.Now(),
				})
			}
		}
	}

	return diffs, nil
}

// CommitChanges creates a commit with the current changes
func (g *Service) CommitChanges(ctx context.Context, repoPath, message string) error {
	// Ensure git is configured for commits
	if err := g.ensureGitConfig(ctx, repoPath); err != nil {
		return fmt.Errorf("failed to configure git: %w", err)
	}

	// Stage all changes
	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = repoPath
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	// Create commit
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commitCmd.Dir = repoPath

	var stderr bytes.Buffer
	var stdout bytes.Buffer
	commitCmd.Stderr = &stderr
	commitCmd.Stdout = &stdout

	if err := commitCmd.Run(); err != nil {
		// Check if there were no changes to commit (can appear in stderr or stdout)
		output := stderr.String() + " " + stdout.String()
		if strings.Contains(output, "nothing to commit") || strings.Contains(output, "working tree clean") {
			return nil
		}
		return fmt.Errorf("failed to commit changes: %v - stderr: %s - stdout: %s", err, stderr.String(), stdout.String())
	}

	return nil
}

// ensureGitConfig ensures git is configured with default user info for commits
func (g *Service) ensureGitConfig(ctx context.Context, repoPath string) error {
	// Check if user.name is already configured
	nameCmd := exec.CommandContext(ctx, "git", "config", "user.name")
	nameCmd.Dir = repoPath
	if err := nameCmd.Run(); err != nil {
		// Set default user.name
		setNameCmd := exec.CommandContext(ctx, "git", "config", "user.name", "Ploy Transflow")
		setNameCmd.Dir = repoPath
		if err := setNameCmd.Run(); err != nil {
			return fmt.Errorf("failed to set git user.name: %w", err)
		}
	}

	// Check if user.email is already configured
	emailCmd := exec.CommandContext(ctx, "git", "config", "user.email")
	emailCmd.Dir = repoPath
	if err := emailCmd.Run(); err != nil {
		// Set default user.email
		setEmailCmd := exec.CommandContext(ctx, "git", "config", "user.email", "mods@ploy.automation")
		setEmailCmd.Dir = repoPath
		if err := setEmailCmd.Run(); err != nil {
			return fmt.Errorf("failed to set git user.email: %w", err)
		}
	}

	return nil
}

// GetCommitHash returns the current commit hash
func (g *Service) GetCommitHash(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// CreateBranch creates a new branch from the current HEAD
func (g *Service) CreateBranch(ctx context.Context, repoPath, branchName string) error {
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", branchName)
	cmd.Dir = repoPath

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}

	return nil
}

// ResetToCommit resets the repository to a specific commit
func (g *Service) ResetToCommit(ctx context.Context, repoPath, commitHash string) error {
	cmd := exec.CommandContext(ctx, "git", "reset", "--hard", commitHash)
	cmd.Dir = repoPath

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reset to commit %s: %w", commitHash, err)
	}

	return nil
}

// GetFileHistory gets the history of changes for specific files
func (g *Service) GetFileHistory(ctx context.Context, repoPath string, files []string) (map[string][]string, error) {
	history := make(map[string][]string)

	for _, file := range files {
		cmd := exec.CommandContext(ctx, "git", "log", "--oneline", "--", file)
		cmd.Dir = repoPath

		output, err := cmd.Output()
		if err != nil {
			continue // File might not have history
		}

		lines := strings.Split(string(output), "\n")
		var commits []string
		for _, line := range lines {
			if line != "" {
				commits = append(commits, line)
			}
		}
		history[file] = commits
	}

	return history, nil
}

// CountChangedFiles counts the number of files changed
func (g *Service) CountChangedFiles(ctx context.Context, repoPath string) (int, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to count changed files: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	count := 0
	for _, line := range lines {
		if line != "" {
			count++
		}
	}

	// Also count staged files
	stagedCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--name-only")
	stagedCmd.Dir = repoPath
	stagedOutput, _ := stagedCmd.Output()

	if len(stagedOutput) > 0 {
		stagedLines := strings.Split(strings.TrimSpace(string(stagedOutput)), "\n")
		for _, line := range stagedLines {
			if line != "" {
				count++
			}
		}
	}

	return count, nil
}

// GetLineChanges counts added and removed lines
func (g *Service) GetLineChanges(ctx context.Context, repoPath string) (added int, removed int, err error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--numstat")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get line changes: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	totalAdded := 0
	totalRemoved := 0

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 3 {
			// Format: added removed filename
			if parts[0] != "-" { // Skip binary files
				var a, r int
				_, _ = fmt.Sscanf(parts[0], "%d", &a)
				_, _ = fmt.Sscanf(parts[1], "%d", &r)
				totalAdded += a
				totalRemoved += r
			}
		}
	}

	// Also count staged changes
	stagedCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--numstat")
	stagedCmd.Dir = repoPath
	stagedOutput, _ := stagedCmd.Output()

	if len(stagedOutput) > 0 {
		stagedLines := strings.Split(string(stagedOutput), "\n")
		for _, line := range stagedLines {
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 3 && parts[0] != "-" {
				var a, r int
				_, _ = fmt.Sscanf(parts[0], "%d", &a)
				_, _ = fmt.Sscanf(parts[1], "%d", &r)
				totalAdded += a
				totalRemoved += r
			}
		}
	}

	return totalAdded, totalRemoved, nil
}
