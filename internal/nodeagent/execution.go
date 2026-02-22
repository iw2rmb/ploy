package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	gitpkg "github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// createGitFetcher initializes a git fetcher for repository operations.
func (r *runController) createGitFetcher() (hydration.GitFetcher, error) {
	cacheDir := os.Getenv("PLOYD_CACHE_HOME")
	return hydration.NewGitFetcher(hydration.GitFetcherOptions{
		CacheDir: cacheDir,
	})
}

func (r *runController) createWorkspaceHydrator(fetcher hydration.GitFetcher) (step.WorkspaceHydrator, error) {
	return step.NewFilesystemWorkspaceHydrator(step.FilesystemWorkspaceHydratorOptions{
		RepoFetcher: fetcher,
	})
}

func (r *runController) createContainerRuntime() (step.ContainerRuntime, error) {
	network := os.Getenv("PLOY_DOCKER_NETWORK")
	return step.NewDockerContainerRuntime(step.DockerContainerRuntimeOptions{
		PullImage: true,
		Network:   network,
	})
}

func (r *runController) createDiffGenerator() step.DiffGenerator {
	return step.NewFilesystemDiffGenerator()
}

// RehydrateWorkspaceFromBaseAndDiffs copies the base clone and applies ordered per-step diffs
// to reconstruct workspace state for multi-step runs.
func RehydrateWorkspaceFromBaseAndDiffs(ctx context.Context, baseClonePath, destWorkspace string, diffs [][]byte) error {
	if strings.TrimSpace(baseClonePath) == "" {
		return fmt.Errorf("baseClonePath is empty")
	}
	if strings.TrimSpace(destWorkspace) == "" {
		return fmt.Errorf("destWorkspace is empty")
	}

	info, err := os.Stat(baseClonePath)
	if err != nil {
		return fmt.Errorf("baseClonePath validation failed: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("baseClonePath is not a directory: %s", baseClonePath)
	}

	if err := copyGitClone(baseClonePath, destWorkspace); err != nil {
		return fmt.Errorf("failed to copy base clone: %w", err)
	}

	for i, gzippedPatch := range diffs {
		if err := applyGzippedPatch(ctx, destWorkspace, gzippedPatch); err != nil {
			return fmt.Errorf("failed to apply diff at index %d: %w", i, err)
		}
	}

	return nil
}

// copyGitClone creates a copy of a git repository from src to dest using rsync.
func copyGitClone(src, dest string) error {
	if _, err := os.Stat(filepath.Join(src, ".git")); err != nil {
		return fmt.Errorf("source is not a git repository: %w", err)
	}

	if _, err := exec.LookPath("rsync"); err != nil {
		return fmt.Errorf("rsync not available for git clone copy: %w", err)
	}

	cmd := exec.Command("rsync", "-a", src+"/", dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// applyGzippedPatch decompresses a gzipped patch and applies it via "git apply".
func applyGzippedPatch(ctx context.Context, workspace string, gzippedPatch []byte) error {
	patch, err := decompressPatch(gzippedPatch)
	if err != nil {
		return fmt.Errorf("failed to decompress patch: %w", err)
	}

	if len(bytes.TrimSpace(patch)) == 0 {
		return nil
	}

	cmd := exec.CommandContext(ctx, "git", "apply")
	cmd.Dir = workspace
	cmd.Stdin = bytes.NewReader(patch)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git apply failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func decompressPatch(gzippedPatch []byte) ([]byte, error) {
	if len(gzippedPatch) == 0 {
		return []byte{}, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(gzippedPatch))
	if err != nil {
		return nil, fmt.Errorf("gzip reader initialization failed: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, fmt.Errorf("gzip decompression failed: %w", err)
	}

	return buf.Bytes(), nil
}

// ensureBaselineCommitForRehydration creates a git commit after applying per-step diffs
// so that "git diff HEAD" in step k captures only step k's changes (not cumulative).
func ensureBaselineCommitForRehydration(ctx context.Context, workspace string, stepIndex types.StepIndex) error {
	message := fmt.Sprintf("Ploy: rehydration baseline for step %.0f", stepIndex)
	_, err := gitpkg.EnsureCommit(ctx, workspace, "ploy-rehydrate", "ploy-rehydrate@ploy.local", message)
	return err
}

// --- Workspace and file utilities ---

const defaultBearerTokenPath = "/etc/ploy/bearer-token"

// bearerTokenPath returns the path to the worker bearer token file,
// overridable for tests via PLOY_NODE_BEARER_TOKEN_PATH.
func bearerTokenPath() string {
	if v := os.Getenv("PLOY_NODE_BEARER_TOKEN_PATH"); v != "" {
		return v
	}
	return defaultBearerTokenPath
}

// createWorkspaceDir creates a temporary workspace directory for a single run.
func createWorkspaceDir() (string, error) {
	base := os.Getenv("PLOYD_CACHE_HOME")
	if base == "" {
		base = os.TempDir()
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	absBase, err := filepath.Abs(base)
	if err == nil {
		base = absBase
	}
	return os.MkdirTemp(base, "ploy-run-*")
}

// listFilesRecursive returns whether directory has any files and a slice of absolute file paths.
func listFilesRecursive(root string) (bool, []string) {
	var out []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			slog.Warn("file walk error", "path", path, "error", err)
			return nil
		}
		if info == nil || info.IsDir() {
			return nil
		}
		out = append(out, path)
		return nil
	})
	return len(out) > 0, out
}
