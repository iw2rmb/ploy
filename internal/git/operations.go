package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// getCurrentBranch gets the current Git branch
func (r *Repository) getCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	branch := strings.TrimSpace(string(output))
	if branch == "HEAD" {
		// We're in a detached HEAD state, try to get the SHA instead
		return "detached", nil
	}

	return branch, nil
}

// getCurrentSHA gets the current Git SHA
func (r *Repository) getCurrentSHA() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current SHA: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetShortSHA gets the short version of the current SHA
func (r *Repository) GetShortSHA() string {
	if len(r.SHA) >= 12 {
		return r.SHA[:12]
	}
	return r.SHA
}

// getRepositoryStatus checks if the repository is clean and has untracked files
func (r *Repository) getRepositoryStatus() (bool, bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		return false, false, fmt.Errorf("failed to get repository status: %w", err)
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

	return !hasChanges && !hasUntracked, hasUntracked, nil
}

// getLastCommit retrieves information about the last commit
func (r *Repository) getLastCommit() (*Commit, error) {
	// Get commit information with format
	cmd := exec.Command("git", "log", "-1", "--format=%H|%s|%an|%ae|%ct|%G?")
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get last commit: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) < 6 {
		return nil, fmt.Errorf("unexpected git log format")
	}

	// Parse timestamp
	timestamp := time.Unix(0, 0)
	if unixTime, err := strconv.ParseInt(parts[4], 10, 64); err == nil {
		timestamp = time.Unix(unixTime, 0)
	}

	// Check GPG signature
	gpgSigned := parts[5] == "G" || parts[5] == "U"

	return &Commit{
		SHA:       parts[0],
		Message:   parts[1],
		Author:    parts[2],
		Email:     parts[3],
		Timestamp: timestamp,
		GPGSigned: gpgSigned,
	}, nil
}

// getRemoteOrigin gets information about the origin remote
func (r *Repository) getRemoteOrigin() (*Remote, error) {
	cmd := exec.Command("git", "remote", "-v")
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get remotes: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "origin") && strings.Contains(line, "fetch") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return &Remote{
					Name: parts[0],
					URL:  parts[1],
					Type: strings.Trim(parts[2], "()"),
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no origin remote found")
}

// getContributors gets list of contributors
func (r *Repository) getContributors() ([]string, error) {
	cmd := exec.Command("git", "shortlog", "-sne")
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var contributors []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Extract email from format: "  123\tJohn Doe <john@example.com>"
		parts := strings.Split(line, "\t")
		if len(parts) > 1 {
			contributors = append(contributors, strings.TrimSpace(parts[1]))
		}
	}

	return contributors, nil
}

// getBranchCount gets the number of branches
func (r *Repository) getBranchCount() (int, error) {
	cmd := exec.Command("git", "branch", "-r")
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" && !strings.Contains(line, "->") {
			count++
		}
	}

	return count, nil
}

// getTagCount gets the number of tags
func (r *Repository) getTagCount() (int, error) {
	cmd := exec.Command("git", "tag", "--list")
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}

	return count, nil
}

// getCommitCount gets the total number of commits
func (r *Repository) getCommitCount() (int, error) {
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	count := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count); err != nil {
		return 0, err
	}

	return count, nil
}

// getFirstCommitTime gets the time of the first commit
func (r *Repository) getFirstCommitTime() (time.Time, error) {
	cmd := exec.Command("git", "log", "--reverse", "--format=%ct", "-1")
	cmd.Dir = r.Path

	output, err := cmd.Output()
	if err != nil {
		return time.Time{}, err
	}

	timestamp := strings.TrimSpace(string(output))
	if unixTime, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
		return time.Unix(unixTime, 0), nil
	}

	return time.Time{}, fmt.Errorf("failed to parse first commit timestamp")
}

// getLastActivityTime gets the time of the last activity
func (r *Repository) getLastActivityTime() (time.Time, error) {
	if r.LastCommit != nil {
		return r.LastCommit.Timestamp, nil
	}
	return time.Time{}, fmt.Errorf("no last commit information")
}
