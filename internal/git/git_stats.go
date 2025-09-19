package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetRepositoryStats returns basic statistics about the repository.
func (g *GitUtils) GetRepositoryStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	if count, err := g.getCommitCount(); err == nil {
		stats["commit_count"] = count
	}
	if contributors, err := g.GetContributors(); err == nil {
		stats["contributor_count"] = len(contributors)
	}
	if branches, err := g.GetBranches(); err == nil {
		stats["branch_count"] = len(branches)
	}
	if tags, err := g.GetTags(); err == nil {
		stats["tag_count"] = len(tags)
	}
	if size, err := g.getRepositorySize(); err == nil {
		stats["size_bytes"] = size
		stats["size_mb"] = size / (1024 * 1024)
	}

	return stats, nil
}

func (g *GitUtils) getCommitCount() (int, error) {
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = g.workingDir

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var count int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count); err != nil {
		return 0, err
	}
	return count, nil
}

func (g *GitUtils) getRepositorySize() (int64, error) {
	var size int64
	err := filepath.Walk(g.workingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
