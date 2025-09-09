package git

import (
	"fmt"
	"os"
	"path/filepath"
)

// NewRepository creates a new repository instance from a directory
func NewRepository(path string) (*Repository, error) {
	// Validate that this is a Git repository
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", path)
	}

	repo := &Repository{Path: path}

	// Extract repository information
	if err := repo.loadRepositoryInfo(); err != nil {
		return nil, fmt.Errorf("failed to load repository info: %w", err)
	}

	return repo, nil
}

// loadRepositoryInfo loads comprehensive repository information
func (r *Repository) loadRepositoryInfo() error {
	var err error

	// Get current branch
	if r.Branch, err = r.getCurrentBranch(); err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Get current SHA
	if r.SHA, err = r.getCurrentSHA(); err != nil {
		return fmt.Errorf("failed to get current SHA: %w", err)
	}

	// Get repository URL
	if r.URL, err = r.getRepositoryURL(); err != nil {
		// URL extraction is not critical, continue with empty URL
		r.URL = ""
	}

	// Check repository status
	if r.IsClean, r.HasUntracked, err = r.getRepositoryStatus(); err != nil {
		return fmt.Errorf("failed to get repository status: %w", err)
	}

	// Get last commit information
	if r.LastCommit, err = r.getLastCommit(); err != nil {
		return fmt.Errorf("failed to get last commit: %w", err)
	}

	// Get remote origin information
	if r.RemoteOrigin, err = r.getRemoteOrigin(); err != nil {
		// Remote origin is optional, continue without error
		r.RemoteOrigin = nil
	}

	return nil
}
