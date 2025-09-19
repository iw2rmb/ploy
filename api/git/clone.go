package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// checkGitAvailable verifies that the git executable is reachable on the host.
func (g *Service) checkGitAvailable() error {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git is not installed or not accessible: %w - Please ensure git is installed on the system", err)
	}
	return nil
}

// CloneRepository clones a Git repository to the specified path.
func (g *Service) CloneRepository(ctx context.Context, repoURL, branch, targetPath string) error {
	if err := g.checkGitAvailable(); err != nil {
		return fmt.Errorf("git dependency check failed: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	normalized := strings.TrimPrefix(branch, "refs/heads/")
	args := []string{"clone", "--depth", "1", "--single-branch"}
	if normalized != "" && normalized != "main" && normalized != "master" {
		args = append(args, "--branch", normalized)
	}
	args = append(args, repoURL, targetPath)

	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %v - %s", err, stderr.String())
	}

	if normalized != "" && normalized != "main" && normalized != "master" {
		checkoutCmd := exec.CommandContext(ctx, "git", "checkout", normalized)
		checkoutCmd.Dir = targetPath
		if err := checkoutCmd.Run(); err != nil {
			fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", normalized)
			fetchCmd.Dir = targetPath
			_ = fetchCmd.Run()
			if err := checkoutCmd.Run(); err != nil {
				return fmt.Errorf("failed to checkout branch %s: %w", normalized, err)
			}
		}
	}

	{
		cfg := exec.CommandContext(ctx, "git", "config", "core.sparseCheckout", "false")
		cfg.Dir = targetPath
		_ = cfg.Run()
		disable := exec.CommandContext(ctx, "git", "sparse-checkout", "disable")
		disable.Dir = targetPath
		_ = disable.Run()
		reset := exec.CommandContext(ctx, "git", "reset", "--hard", "HEAD")
		reset.Dir = targetPath
		_ = reset.Run()
	}

	if fi, err := os.Stat(filepath.Join(targetPath, ".git")); err != nil || !fi.IsDir() {
		return fmt.Errorf("git clone produced no .git directory at %s", targetPath)
	}
	if entries, err := os.ReadDir(targetPath); err == nil {
		nonMeta := 0
		for _, e := range entries {
			if e.Name() == ".git" {
				continue
			}
			nonMeta++
		}
		if nonMeta == 0 {
			return fmt.Errorf("git clone empty working tree at %s (no files besides .git)", targetPath)
		}
	}

	return nil
}
