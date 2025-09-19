package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitUtils provides enhanced Git utility functions.
type GitUtils struct {
	workingDir string
}

// NewGitUtils creates a new GitUtils instance for the specified directory.
func NewGitUtils(workingDir string) *GitUtils {
	return &GitUtils{workingDir: workingDir}
}

// GetSHA returns the current Git SHA (full or short version).
func (g *GitUtils) GetSHA(short bool) (string, error) {
	args := []string{"rev-parse", "HEAD"}
	if short {
		args = []string{"rev-parse", "--short=12", "HEAD"}
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = g.workingDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git SHA: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetShortSHA returns the short version of the current Git SHA.
func (g *GitUtils) GetShortSHA() string {
	sha, _ := g.GetSHA(true)
	return sha
}

// GetBranch returns the current Git branch.
func (g *GitUtils) GetBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = g.workingDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git branch: %w", err)
	}

	branch := strings.TrimSpace(string(output))
	if branch == "HEAD" {
		return "detached", nil
	}

	return branch, nil
}

// IsGitRepository checks if the directory is a Git repository.
func (g *GitUtils) IsGitRepository() bool {
	gitDir := filepath.Join(g.workingDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}

	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = g.workingDir

	return cmd.Run() == nil
}

// ValidateWorkingDirectory checks if the working directory is valid for Git operations.
func (g *GitUtils) ValidateWorkingDirectory() error {
	if _, err := os.Stat(g.workingDir); os.IsNotExist(err) {
		return fmt.Errorf("working directory does not exist: %s", g.workingDir)
	}

	if !g.IsGitRepository() {
		return fmt.Errorf("directory is not a Git repository: %s", g.workingDir)
	}

	return nil
}
