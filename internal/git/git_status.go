package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// GetStatus returns the Git status (clean, dirty, untracked files).
func (g *GitUtils) GetStatus() (bool, bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = g.workingDir

	output, err := cmd.Output()
	if err != nil {
		return false, false, fmt.Errorf("failed to get git status: %w", err)
	}

	statusLines := strings.Split(strings.TrimSpace(string(output)), "\n")
	hasUntracked := false
	hasChanges := false

	for _, line := range statusLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "??") {
			hasUntracked = true
		} else {
			hasChanges = true
		}
	}

	isClean := !hasChanges && !hasUntracked
	return isClean, hasUntracked, nil
}

// GetCommitInfo returns information about the current commit.
func (g *GitUtils) GetCommitInfo() (map[string]string, error) {
	cmd := exec.Command("git", "log", "-1", "--format=%H|%s|%an|%ae|%ad|%G?", "--date=iso")
	cmd.Dir = g.workingDir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit info: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) < 6 {
		return nil, fmt.Errorf("unexpected git log format")
	}

	return map[string]string{
		"sha":     parts[0],
		"message": parts[1],
		"author":  parts[2],
		"email":   parts[3],
		"date":    parts[4],
		"gpg_signed": func() string {
			if parts[5] == "G" || parts[5] == "U" {
				return "true"
			}
			return "false"
		}(),
	}, nil
}

// GetContributors returns a list of contributors to the repository.
func (g *GitUtils) GetContributors() ([]string, error) {
	cmd := exec.Command("git", "shortlog", "-sne")
	cmd.Dir = g.workingDir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get contributors: %w", err)
	}

	var contributors []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) > 1 {
			contributors = append(contributors, strings.TrimSpace(parts[1]))
		}
	}

	return contributors, nil
}

// GetTags returns a list of Git tags.
func (g *GitUtils) GetTags() ([]string, error) {
	cmd := exec.Command("git", "tag", "-l", "--sort=-version:refname")
	cmd.Dir = g.workingDir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get tags: %w", err)
	}

	var tags []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if tag := strings.TrimSpace(line); tag != "" {
			tags = append(tags, tag)
		}
	}

	return tags, nil
}

// GetBranches returns a list of Git branches.
func (g *GitUtils) GetBranches() ([]string, error) {
	cmd := exec.Command("git", "branch", "-r")
	cmd.Dir = g.workingDir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get branches: %w", err)
	}

	var branches []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		branch := strings.TrimSpace(line)
		if branch != "" && !strings.Contains(branch, "->") {
			branch = strings.TrimPrefix(branch, "origin/")
			branches = append(branches, branch)
		}
	}

	return branches, nil
}
