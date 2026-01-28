package step

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Stub implementations for workflow runtime step package.
// These are minimal placeholders to allow compilation until full implementation.

// (artifact publisher removed — artifacts are uploaded by the node agent)

// GitFetcher is the interface for fetching git repositories.
type GitFetcher interface {
	Fetch(ctx context.Context, repo *contracts.RepoMaterialization, dest string) error
}

// FilesystemWorkspaceHydratorOptions holds configuration for workspace hydrator.
type FilesystemWorkspaceHydratorOptions struct {
	RepoFetcher GitFetcher
}

// WorkspaceHydrator prepares a workspace for execution.
type WorkspaceHydrator interface {
	Hydrate(ctx context.Context, manifest contracts.StepManifest, workspace string) error
}

type filesystemWorkspaceHydrator struct {
	fetcher GitFetcher
}

// NewFilesystemWorkspaceHydrator creates a new workspace hydrator.
func NewFilesystemWorkspaceHydrator(opts FilesystemWorkspaceHydratorOptions) (WorkspaceHydrator, error) {
	if opts.RepoFetcher == nil {
		return nil, errors.New("repo fetcher is required")
	}
	return &filesystemWorkspaceHydrator{fetcher: opts.RepoFetcher}, nil
}

// Hydrate prepares the workspace by fetching repository sources as needed.
func (h *filesystemWorkspaceHydrator) Hydrate(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
	// Process each input that has repository hydration configured.
	for _, input := range manifest.Inputs {
		if input.Hydration != nil && input.Hydration.Repo != nil {
			// Fetch the repository into the workspace at the input's mount path.
			// For now, we fetch directly into workspace; future work may use mount paths.
			if err := h.fetcher.Fetch(ctx, input.Hydration.Repo, workspace); err != nil {
				return fmt.Errorf("failed to hydrate input %s: %w", input.Name, err)
			}
		}
	}
	return nil
}

// DockerContainerRuntimeOptions holds configuration for Docker runtime.
type DockerContainerRuntimeOptions struct {
	// PullImage controls whether the runtime ensures the image is available
	// (by pulling it only when missing) before container creation.
	PullImage bool
	// Network is optional Docker network name (empty => default bridge).
	Network string
}

// ContainerRuntime executes containers.
type ContainerRuntime interface {
	Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error)
	Start(ctx context.Context, handle ContainerHandle) error
	Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error)
	Logs(ctx context.Context, handle ContainerHandle) ([]byte, error)
	Remove(ctx context.Context, handle ContainerHandle) error
}

// FilesystemDiffGeneratorOptions holds configuration for diff generator.
type FilesystemDiffGeneratorOptions struct{}

// DiffGenerator generates diffs between states.
type DiffGenerator interface {
	Generate(ctx context.Context, workspace string) ([]byte, error)
	// GenerateBetween computes a diff between two directories (base and modified).
	// Used by C2 to capture pre-mod healing changes (base clone → healed workspace).
	GenerateBetween(ctx context.Context, baseDir, modifiedDir string) ([]byte, error)
}

type filesystemDiffGenerator struct{}

// NewFilesystemDiffGenerator creates a new diff generator.
func NewFilesystemDiffGenerator(opts FilesystemDiffGeneratorOptions) DiffGenerator {
	_ = opts
	return &filesystemDiffGenerator{}
}

// Generate produces a unified diff of all changes in the workspace using git diff.
func (d *filesystemDiffGenerator) Generate(ctx context.Context, workspace string) ([]byte, error) {
	return generateGitDiff(ctx, workspace)
}

// GenerateBetween computes a unified diff between two directories.
// Uses git diff --no-index to compare arbitrary directories (not requiring a git repo).
func (d *filesystemDiffGenerator) GenerateBetween(ctx context.Context, baseDir, modifiedDir string) ([]byte, error) {
	return generateGitDiffBetween(ctx, baseDir, modifiedDir)
}

// generateGitDiff runs git diff to capture all changes in the workspace.
func generateGitDiff(ctx context.Context, workspace string) ([]byte, error) {
	// Run git diff to get unified diff of all changes (staged and unstaged).
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	cmd.Dir = workspace

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// git diff returns exit code 0 even when there are diffs.
		// Only fail if there's an actual error (not just "no diff").
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git diff cancelled: %w", ctx.Err())
		}
		// If stderr has content, surface it; otherwise propagate the run error.
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("git diff failed: %s", strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	return stdout.Bytes(), nil
}

// generateGitDiffBetween computes a unified diff between two directories using git diff --no-index.
// This works even when neither directory is a git repository.
// Used by C2 to compute pre-mod healing diffs (base clone vs healed workspace).
func generateGitDiffBetween(ctx context.Context, baseDir, modifiedDir string) ([]byte, error) {
	// Use git diff --no-index to compare two arbitrary directories.
	// Note: git diff --no-index returns exit code 1 when there ARE differences (not an error).
	// Use --no-prefix to get raw paths, then normalize them for portable diffs.
	cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--no-prefix", baseDir, modifiedDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Check for context cancellation first.
	if ctx.Err() != nil {
		return nil, fmt.Errorf("git diff --no-index cancelled: %w", ctx.Err())
	}

	// git diff --no-index exit codes:
	// 0: no differences
	// 1: differences found (this is success, not an error)
	// >1: error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				// Exit code 1 means differences were found - this is the expected case.
				// Normalize paths to produce portable diffs.
				return normalizeDiffPaths(stdout.Bytes(), baseDir, modifiedDir), nil
			}
		}
		// Actual error occurred.
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("git diff --no-index failed: %s", strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("git diff --no-index failed: %w", err)
	}

	// Exit code 0: no differences.
	return stdout.Bytes(), nil
}

// normalizeDiffPaths rewrites git diff output to use standard a/ and b/ prefixes
// with relative paths. git diff --no-index --no-prefix produces paths like:
//
//	diff --git private/tmp/base/file private/tmp/modified/file
//	--- private/tmp/base/file
//	+++ private/tmp/modified/file
//
// (Note: git strips the leading / from absolute paths in --no-prefix mode)
//
// This function normalizes them to:
//
//	diff --git a/file b/file
//	--- a/file
//	+++ b/file
//
// This enables git apply -p1 to work correctly during rehydration.
// Additionally, it filters out .git/ directory changes which git apply rejects.
func normalizeDiffPaths(diff []byte, baseDir, modifiedDir string) []byte {
	// Git strips leading / from paths in --no-prefix mode, so we need to match
	// both with and without the leading slash.
	baseDir = strings.TrimPrefix(baseDir, "/")
	modifiedDir = strings.TrimPrefix(modifiedDir, "/")

	// Ensure paths end with / for clean replacement.
	if !strings.HasSuffix(baseDir, "/") {
		baseDir += "/"
	}
	if !strings.HasSuffix(modifiedDir, "/") {
		modifiedDir += "/"
	}

	result := string(diff)
	// Replace base directory paths with a/ prefix.
	result = strings.ReplaceAll(result, baseDir, "a/")
	// Replace modified directory paths with b/ prefix.
	result = strings.ReplaceAll(result, modifiedDir, "b/")

	// Filter out .git/ directory changes - git apply rejects patches to .git/ internals.
	return filterGitDir([]byte(result))
}

// filterGitDir removes diff hunks that modify .git/ directory contents.
// git apply rejects patches that attempt to modify .git/ internals (e.g., .git/index),
// so we strip them from the diff output.
func filterGitDir(diff []byte) []byte {
	lines := strings.Split(string(diff), "\n")
	var filtered []string
	skip := false

	for _, line := range lines {
		// Each file in a diff starts with "diff --git ..."
		if strings.HasPrefix(line, "diff --git") {
			// Check if this diff hunk is for a .git/ file
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

// Runner executes workflow steps.
//
// # Execution Stages (Pre-mod Gate per Call)
//
// Runner.Run processes each step call through the following stages in order:
//
//  1. Hydration — Prepare the workspace by fetching repository sources via
//     WorkspaceHydrator. Errors here abort the run immediately.
//
//  2. Pre-mod Build Gate — When Gate is enabled (Manifest.Gate.Enabled), run static validation on the
//     workspace before executing the mod container. If the gate fails,
//     Runner.Run returns ErrBuildGateFailed without executing container
//     stages. The node agent orchestration layer handles healing when
//     configured; Runner itself does not perform healing.
//
//  3. Container Execution — Create, start, and wait on the container via
//     ContainerRuntime. Logs are forwarded to LogWriter if present.
//     Container is removed after completion unless retention is requested.
//
//  4. Diff Generation — Generate a unified diff of workspace changes via
//     DiffGenerator. The diff is not published here; the node agent
//     uploads artifacts independently.
//
// This contract establishes the baseline for adding post-mod gate helpers
// in subsequent phases. Each call to Run executes exactly one pre-mod gate
// (when enabled) before a single mod container.
//
// # Gate Ownership Contract
//
// Runner supports an optional pre-mod gate when Manifest.Gate.Enabled=true.
// This capability exists for direct invocations (e.g., standalone testing)
// where Runner manages its own gate lifecycle.
//
// However, nodeagent step execution MUST pass manifests with Gate.Enabled=false.
// The nodeagent orchestration layer owns all gate lifecycle management via
// runGateWithHealing, which handles:
//   - A single pre-run gate before the step loop begins.
//   - Per-step post-mod gates after each container execution.
//   - Healing retries when gates fail and healing is configured.
//
// Passing Gate.Enabled=true from nodeagent would cause duplicate pre-mod gates
// (one from runGateWithHealing, one from Runner.Run) and break the single-gate-
// per-run invariant. The nodeagent is the authoritative gate orchestrator.
type Runner struct {
	Workspace  WorkspaceHydrator
	Containers ContainerRuntime
	Diffs      DiffGenerator
	Gate       GateExecutor
	LogWriter  io.Writer // Optional: streams logs to server as gzipped chunks.
}

// Request describes a step execution request.
type Request struct {
	// RunID threads the workflow run identifier for correlation/labels.
	// Container labels and telemetry use this value via LabelRunID.
	RunID     types.RunID
	Manifest  contracts.StepManifest
	Workspace string
	OutDir    string
	// InDir is an optional read-only directory mounted at /in for cross-phase inputs.
	InDir string
}

// Result contains the outcome of a step execution.
type Result struct {
	ExitCode int
	// Per-stage timings captured during execution.
	Timings   StageTiming
	BuildGate *contracts.BuildGateStageMetadata
}

// StageTiming captures duration of each execution stage.
type StageTiming struct {
	HydrationDuration types.Duration
	ExecutionDuration types.Duration
	BuildGateDuration types.Duration
	DiffDuration      types.Duration
	PublishDuration   types.Duration
	TotalDuration     types.Duration
}

// ErrBuildGateFailed is returned when the pre-mod Build Gate fails
// and no healing is configured to continue.
var ErrBuildGateFailed = errors.New("build gate failed")

// Run executes a step and returns the result.
func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	totalStart := time.Now()
	var result Result

	// Stage 1: Hydrate workspace.
	hydrationStart := time.Now()
	if r.Workspace != nil {
		if err := r.Workspace.Hydrate(ctx, req.Manifest, req.Workspace); err != nil {
			return Result{}, fmt.Errorf("workspace hydration failed: %w", err)
		}
	}
	result.Timings.HydrationDuration = types.Duration(time.Since(hydrationStart))

	// Stage 2: Pre-mod Build Gate validation.
	// Run the Build Gate before executing the mod container to fail fast if the codebase doesn't build.
	gateStart := time.Now()
	gateSpec := req.Manifest.Gate
	if r.Gate != nil && gateSpec != nil && gateSpec.Enabled {
		gateMetadata, err := r.Gate.Execute(ctx, gateSpec, req.Workspace)
		if err != nil {
			return Result{}, fmt.Errorf("build gate execution failed: %w", err)
		}
		result.BuildGate = gateMetadata

		// If the pre-mod gate fails and no healing is configured, fail immediately.
		// Check if gate passed by inspecting StaticChecks.
		gatePassed := false
		if len(gateMetadata.StaticChecks) > 0 {
			gatePassed = gateMetadata.StaticChecks[0].Passed
		}
		if !gatePassed {
			// Gate failed. Always return error; node agent orchestration layer
			// will handle healing if configured.
			result.Timings.BuildGateDuration = types.Duration(time.Since(gateStart))
			result.Timings.TotalDuration = types.Duration(time.Since(totalStart))
			return result, fmt.Errorf("%w: %s", ErrBuildGateFailed, "pre-mod validation failed")
		}
	}
	result.Timings.BuildGateDuration = types.Duration(time.Since(gateStart))

	// Stage 3: Execute container via configured runtime (real execution when available).
	executionStart := time.Now()
	if r.Containers == nil {
		// Backward-compatible fallback: simulate execution when no runtime is configured
		if r.LogWriter != nil {
			_, _ = fmt.Fprintf(r.LogWriter, "Starting execution for manifest %s\n", req.Manifest.ID)
		}
		result.ExitCode = 0
		result.Timings.ExecutionDuration = types.Duration(time.Since(executionStart))
	} else {
		// Build container spec from manifest and workspace path plus optional /out and /in mounts.
		spec, err := buildContainerSpec(req.RunID, req.Manifest, req.Workspace, req.OutDir, req.InDir)
		if err != nil {
			return Result{}, fmt.Errorf("build container spec: %w", err)
		}
		handle, err := r.Containers.Create(ctx, spec)
		if err != nil {
			return Result{}, fmt.Errorf("container create failed: %w", err)
		}
		if err := r.Containers.Start(ctx, handle); err != nil {
			return Result{}, fmt.Errorf("container start failed: %w", err)
		}
		cRes, err := r.Containers.Wait(ctx, handle)
		if err != nil {
			return Result{}, fmt.Errorf("container wait failed: %w", err)
		}
		// Fetch logs and forward to LogWriter if present.
		if r.LogWriter != nil {
			if logs, err := r.Containers.Logs(ctx, handle); err == nil && len(logs) > 0 {
				_, _ = r.LogWriter.Write(logs)
			}
		}
		result.ExitCode = cRes.ExitCode
		result.Timings.ExecutionDuration = types.Duration(time.Since(executionStart))

		// Explicitly remove the container unless retention is requested.
		if !req.Manifest.Retention.RetainContainer {
			// Best-effort cleanup; ignore remove errors.
			_ = r.Containers.Remove(ctx, handle)
		}
	}

	// Stage 4: Generate diff (best-effort; publishing handled by node agent).
	diffStart := time.Now()
	if r.Diffs != nil {
		diffBytes, err := r.Diffs.Generate(ctx, req.Workspace)
		if err != nil {
			return Result{}, fmt.Errorf("diff generation failed: %w", err)
		}
		_ = diffBytes // Node agent independently uploads diffs; avoid duplicate publish with stubbed publisher.
	}
	result.Timings.DiffDuration = types.Duration(time.Since(diffStart))

	// Stage 5: (removed) Artifact publishing is performed by the node agent.

	result.Timings.TotalDuration = types.Duration(time.Since(totalStart))
	return result, nil
}
