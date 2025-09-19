package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

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

// GetDiff captures the diff of changes in the repository.
func (g *Service) GetDiff(ctx context.Context, repoPath string) ([]DiffCapture, error) {
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
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		file := parts[1]
		diff := DiffCapture{File: file, Timestamp: time.Now()}

		switch status {
		case "M", "MM":
			diff.Type = "modified"
			diffCmd := exec.CommandContext(ctx, "git", "diff", file)
			diffCmd.Dir = repoPath
			diffOutput, _ := diffCmd.Output()
			diff.UnifiedDiff = string(diffOutput)
		case "A", "AM":
			diff.Type = "added"
			content, _ := os.ReadFile(filepath.Join(repoPath, file))
			diff.After = string(content)
			diff.UnifiedDiff = fmt.Sprintf("+++ %s\n%s", file, diff.After)
		case "D":
			diff.Type = "deleted"
			diffCmd := exec.CommandContext(ctx, "git", "diff", "HEAD", file)
			diffCmd.Dir = repoPath
			diffOutput, _ := diffCmd.Output()
			diff.UnifiedDiff = string(diffOutput)
		case "??":
			diff.Type = "added"
			content, _ := os.ReadFile(filepath.Join(repoPath, file))
			diff.After = string(content)
			diff.UnifiedDiff = fmt.Sprintf("+++ %s (new file)\n%s", file, diff.After)
		}

		diffs = append(diffs, diff)
	}

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
			if len(parts) < 2 {
				continue
			}
			diffCmd := exec.CommandContext(ctx, "git", "diff", "--cached", parts[1])
			diffCmd.Dir = repoPath
			diffOutput, _ := diffCmd.Output()
			diffs = append(diffs, DiffCapture{File: parts[1], Type: "modified", UnifiedDiff: string(diffOutput), Timestamp: time.Now()})
		}
	}

	return diffs, nil
}
