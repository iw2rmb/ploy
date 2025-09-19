package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GetFileHistory gets the history of changes for specific files.
func (g *Service) GetFileHistory(ctx context.Context, repoPath string, files []string) (map[string][]string, error) {
	history := make(map[string][]string)

	for _, file := range files {
		cmd := exec.CommandContext(ctx, "git", "log", "--oneline", "--", file)
		cmd.Dir = repoPath
		output, err := cmd.Output()
		if err != nil {
			continue
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

// CountChangedFiles counts the number of files changed.
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

// GetLineChanges counts added and removed lines.
func (g *Service) GetLineChanges(ctx context.Context, repoPath string) (added int, removed int, err error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--numstat")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get line changes: %w", err)
	}

	parse := func(output string) (int, int) {
		lines := strings.Split(output, "\n")
		var totalAdded, totalRemoved int
		for _, line := range lines {
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
		return totalAdded, totalRemoved
	}

	totalAdded, totalRemoved := parse(string(output))

	stagedCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--numstat")
	stagedCmd.Dir = repoPath
	stagedOutput, _ := stagedCmd.Output()
	if len(stagedOutput) > 0 {
		a, r := parse(string(stagedOutput))
		totalAdded += a
		totalRemoved += r
	}

	return totalAdded, totalRemoved, nil
}
