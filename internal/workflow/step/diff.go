package step

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// PatchStats holds line-level statistics derived from a unified diff.
type PatchStats struct {
	FilesChanged int
	LinesAdded   int
	LinesRemoved int
}

// CountPatchStats parses a unified diff and returns file and line delta counts.
// It counts `+` lines (excluding `+++ ` file headers) as additions and `-` lines
// (excluding `--- ` file headers) as removals. Each `diff --git` or `diff --no-index`
// header marks one changed file.
func CountPatchStats(patchBytes []byte) PatchStats {
	if len(patchBytes) == 0 {
		return PatchStats{}
	}
	var stats PatchStats
	scanner := bufio.NewScanner(bytes.NewReader(patchBytes))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "diff --"):
			stats.FilesChanged++
		case strings.HasPrefix(line, "+++ "):
			// file header — skip
		case strings.HasPrefix(line, "--- "):
			// file header — skip
		case strings.HasPrefix(line, "+"):
			stats.LinesAdded++
		case strings.HasPrefix(line, "-"):
			stats.LinesRemoved++
		}
	}
	return stats
}

type filesystemDiffGenerator struct{}

func (d *filesystemDiffGenerator) Generate(ctx context.Context, workspace string) ([]byte, error) {
	return generateGitDiff(ctx, workspace)
}

func (d *filesystemDiffGenerator) GenerateBetween(ctx context.Context, baseDir, modifiedDir string) ([]byte, error) {
	return generateGitDiffBetween(ctx, baseDir, modifiedDir)
}

// NewFilesystemDiffGenerator creates a DiffGenerator backed by git diff.
func NewFilesystemDiffGenerator() DiffGenerator {
	return &filesystemDiffGenerator{}
}

// generateGitDiff runs git diff to capture all changes in the workspace.
func generateGitDiff(ctx context.Context, workspace string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	cmd.Dir = workspace

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git diff cancelled: %w", ctx.Err())
		}
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("git diff failed: %s", strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	return stdout.Bytes(), nil
}

// generateGitDiffBetween computes a unified diff between two directories using git diff --no-index.
// This works even when neither directory is a git repository.
func generateGitDiffBetween(ctx context.Context, baseDir, modifiedDir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--no-prefix", baseDir, modifiedDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if ctx.Err() != nil {
		return nil, fmt.Errorf("git diff --no-index cancelled: %w", ctx.Err())
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return normalizeDiffPaths(stdout.Bytes(), baseDir, modifiedDir), nil
			}
		}
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("git diff --no-index failed: %s", strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("git diff --no-index failed: %w", err)
	}

	return stdout.Bytes(), nil
}

// normalizeDiffPaths rewrites git diff output to use standard a/ and b/ prefixes
// with relative paths.
func normalizeDiffPaths(diff []byte, baseDir, modifiedDir string) []byte {
	baseDir = strings.TrimPrefix(baseDir, "/")
	modifiedDir = strings.TrimPrefix(modifiedDir, "/")

	if !strings.HasSuffix(baseDir, "/") {
		baseDir += "/"
	}
	if !strings.HasSuffix(modifiedDir, "/") {
		modifiedDir += "/"
	}

	result := string(diff)
	result = strings.ReplaceAll(result, baseDir, "a/")
	result = strings.ReplaceAll(result, modifiedDir, "b/")

	return filterGitDir([]byte(result))
}

// filterGitDir removes diff hunks that modify .git/ directory contents.
func filterGitDir(diff []byte) []byte {
	lines := strings.Split(string(diff), "\n")
	var filtered []string
	skip := false

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			skip = strings.Contains(line, "/.git/") ||
				strings.Contains(line, "a/.git/") ||
				strings.Contains(line, "b/.git/")
		}
		if !skip {
			filtered = append(filtered, line)
		}
	}

	return []byte(strings.Join(filtered, "\n"))
}
