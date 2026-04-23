package step

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
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
// (excluding `--- ` file headers) as removals. Each `diff --*` header marks one
// changed file.
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

// NewFilesystemDiffGenerator creates a DiffGenerator backed by a temporary git
// index snapshot.
func NewFilesystemDiffGenerator() DiffGenerator {
	return &filesystemDiffGenerator{}
}

// generateGitDiff captures workspace changes using a temporary git index:
// `git add -A -- .` then `git diff --cached HEAD`.
// This respects `.gitignore`, includes non-ignored new files, and does not
// mutate the repository's real index.
func generateGitDiff(ctx context.Context, workspace string) ([]byte, error) {
	indexFile, err := os.CreateTemp("", "ploy-diff-index-*")
	if err != nil {
		return nil, fmt.Errorf("create temp index: %w", err)
	}
	indexPath := indexFile.Name()
	if closeErr := indexFile.Close(); closeErr != nil {
		_ = os.Remove(indexPath)
		return nil, fmt.Errorf("close temp index: %w", closeErr)
	}
	if err := os.Remove(indexPath); err != nil {
		return nil, fmt.Errorf("remove temp index placeholder: %w", err)
	}
	defer func() {
		_ = os.Remove(indexPath)
	}()

	env := []string{"GIT_INDEX_FILE=" + indexPath}
	if err := runGitCommandWithEnv(ctx, workspace, env, "read-tree", "HEAD"); err != nil {
		return nil, fmt.Errorf("git read-tree failed: %w", err)
	}
	if err := runGitCommandWithEnv(ctx, workspace, env, "add", "-A", "--", "."); err != nil {
		return nil, fmt.Errorf("git add failed: %w", err)
	}

	diff, err := runGitOutputWithEnv(ctx, workspace, env, "diff", "--cached", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git diff --cached failed: %w", err)
	}
	return diff, nil
}

func runGitCommandWithEnv(ctx context.Context, workspace string, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), env...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("cancelled: %w", ctx.Err())
		}
		if stderr.Len() > 0 {
			return fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

func runGitOutputWithEnv(ctx context.Context, workspace string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("cancelled: %w", ctx.Err())
		}
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}
