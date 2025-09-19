package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	lineCounts := collectLineCounts(ctx, repoPath)

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

		if stat, ok := lineCounts[file]; ok {
			diff.LinesAdded = stat.added
			diff.LinesRemoved = stat.removed
		}

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
			if diff.LinesAdded == 0 {
				diff.LinesAdded = countFileLines(content)
			}
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
			if diff.LinesAdded == 0 {
				diff.LinesAdded = countFileLines(content)
			}
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
			file := parts[1]
			diffCmd := exec.CommandContext(ctx, "git", "diff", "--cached", file)
			diffCmd.Dir = repoPath
			diffOutput, _ := diffCmd.Output()
			diff := DiffCapture{File: file, Type: "modified", UnifiedDiff: string(diffOutput), Timestamp: time.Now()}
			if stat, ok := lineCounts[file]; ok {
				diff.LinesAdded = stat.added
				diff.LinesRemoved = stat.removed
			}
			diffs = append(diffs, diff)
		}
	}

	return diffs, nil
}

type lineStat struct {
	added   int
	removed int
}

func collectLineCounts(ctx context.Context, repoPath string) map[string]lineStat {
	counts := make(map[string]lineStat)

	parse := func(args ...string) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoPath
		output, err := cmd.Output()
		if err != nil {
			return
		}
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) < 3 {
				continue
			}
			added, err1 := strconv.Atoi(parts[0])
			removed, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil {
				continue
			}
			path := parts[2]
			stat := counts[path]
			stat.added += added
			stat.removed += removed
			counts[path] = stat
		}
	}

	parse("diff", "--numstat")
	parse("diff", "--cached", "--numstat")

	return counts
}

func countFileLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	lines := bytes.Count(content, []byte{'\n'})
	if content[len(content)-1] != '\n' {
		lines++
	}
	if lines == 0 {
		lines = 1
	}
	return lines
}
