package arf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitOperations provides Git functionality for ARF transformations
type GitOperations struct {
	workDir string
}

// NewGitOperations creates a new Git operations handler
func NewGitOperations(workDir string) *GitOperations {
	return &GitOperations{
		workDir: workDir,
	}
}

// CreateBranchAndCheckout creates a new branch and switches to it (or just checks out if exists)
func (g *GitOperations) CreateBranchAndCheckout(ctx context.Context, repoPath, branchName string) error {
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

// PushBranch pushes the current HEAD to the given remote URL and branch (HTTPS with token recommended)
func (g *GitOperations) PushBranch(ctx context.Context, repoPath, remoteURL, branchName string) error {
	// If an access token is available, embed it into the remote URL for HTTPS auth
	remoteURL = g.authenticatedRemoteURL(remoteURL)
	// Set remote named 'origin' to provided URL
	rm := exec.CommandContext(ctx, "git", "remote", "remove", "origin")
	rm.Dir = repoPath
	_ = rm.Run() // ignore error; remote may not exist

	add := exec.CommandContext(ctx, "git", "remote", "add", "origin", remoteURL)
	add.Dir = repoPath
	if err := add.Run(); err != nil {
		return fmt.Errorf("failed to set remote origin: %w", err)
	}

	// Push branch with upstream set
	push := exec.CommandContext(ctx, "git", "push", "-u", "origin", branchName)
	push.Dir = repoPath
	var stderr bytes.Buffer
	push.Stderr = &stderr
	if err := push.Run(); err != nil {
		rc := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ProcessState != nil {
			rc = ee.ProcessState.ExitCode()
		}
		return fmt.Errorf("git push failed: rc=%d: %v - %s", rc, err, stderr.String())
	}
	return nil
}

// authenticatedRemoteURL injects credentials into the remote URL when possible.
// For GitLab, using username "oauth2" with the token as password works for PATs
// and project/group access tokens. Only applies to http/https URLs.
func (g *GitOperations) authenticatedRemoteURL(remote string) string {
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
func (g *GitOperations) checkGitAvailable() error {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git is not installed or not accessible: %w - Please ensure git is installed on the system", err)
	}
	return nil
}

// CloneRepository clones a Git repository to the specified path
func (g *GitOperations) CloneRepository(ctx context.Context, repoURL, branch, targetPath string) error {
	// Check if git is available
	if err := g.checkGitAvailable(); err != nil {
		return fmt.Errorf("git dependency check failed: %w", err)
	}

	// Create parent directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Build clone command
	args := []string{"clone", "--depth", "1", "--single-branch"}
	if branch != "" && branch != "main" && branch != "master" {
		args = append(args, "--branch", branch)
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
	if branch != "" && branch != "main" && branch != "master" {
		checkoutCmd := exec.CommandContext(ctx, "git", "checkout", branch)
		checkoutCmd.Dir = targetPath
		if err := checkoutCmd.Run(); err != nil {
			// Try to fetch the branch first
			fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", branch)
			fetchCmd.Dir = targetPath
			fetchCmd.Run() // Ignore fetch errors

			// Try checkout again
			if err := checkoutCmd.Run(); err != nil {
				return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
			}
		}
	}

	return nil
}

// GetDiff captures the diff of changes in the repository
func (g *GitOperations) GetDiff(ctx context.Context, repoPath string) ([]DiffCapture, error) {
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
func (g *GitOperations) CommitChanges(ctx context.Context, repoPath, message string) error {
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
func (g *GitOperations) ensureGitConfig(ctx context.Context, repoPath string) error {
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
		setEmailCmd := exec.CommandContext(ctx, "git", "config", "user.email", "transflow@ploy.automation")
		setEmailCmd.Dir = repoPath
		if err := setEmailCmd.Run(); err != nil {
			return fmt.Errorf("failed to set git user.email: %w", err)
		}
	}

	return nil
}

// GetCommitHash returns the current commit hash
func (g *GitOperations) GetCommitHash(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// CreateBranch creates a new branch from the current HEAD
func (g *GitOperations) CreateBranch(ctx context.Context, repoPath, branchName string) error {
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", branchName)
	cmd.Dir = repoPath

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}

	return nil
}

// ResetToCommit resets the repository to a specific commit
func (g *GitOperations) ResetToCommit(ctx context.Context, repoPath, commitHash string) error {
	cmd := exec.CommandContext(ctx, "git", "reset", "--hard", commitHash)
	cmd.Dir = repoPath

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reset to commit %s: %w", commitHash, err)
	}

	return nil
}

// GetFileHistory gets the history of changes for specific files
func (g *GitOperations) GetFileHistory(ctx context.Context, repoPath string, files []string) (map[string][]string, error) {
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
func (g *GitOperations) CountChangedFiles(ctx context.Context, repoPath string) (int, error) {
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
func (g *GitOperations) GetLineChanges(ctx context.Context, repoPath string) (added int, removed int, err error) {
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
				fmt.Sscanf(parts[0], "%d", &a)
				fmt.Sscanf(parts[1], "%d", &r)
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
				fmt.Sscanf(parts[0], "%d", &a)
				fmt.Sscanf(parts[1], "%d", &r)
				totalAdded += a
				totalRemoved += r
			}
		}
	}

	return totalAdded, totalRemoved, nil
}
