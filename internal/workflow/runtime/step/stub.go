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
	// PullImage controls whether the runtime pulls the image before start.
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

// Runner executes workflow steps.
type Runner struct {
	Workspace  WorkspaceHydrator
	Containers ContainerRuntime
	Diffs      DiffGenerator
	Gate       GateExecutor
	LogWriter  io.Writer // Optional: streams logs to server as gzipped chunks.
}

// Request describes a step execution request.
type Request struct {
	// TicketID threads the workflow ticket identifier for correlation/labels.
	TicketID  types.TicketID
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
	HydrationDuration time.Duration
	ExecutionDuration time.Duration
	BuildGateDuration time.Duration
	DiffDuration      time.Duration
	PublishDuration   time.Duration
	TotalDuration     time.Duration
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
	result.Timings.HydrationDuration = time.Since(hydrationStart)

	// Stage 2: Pre-mod Build Gate validation.
	// Run the Build Gate before executing the mod container to fail fast if the codebase doesn't build.
	gateStart := time.Now()
	gateSpec := req.Manifest.Gate
	//lint:ignore SA1019 Backward compatibility: support deprecated Shift by mapping to Gate.
	if gateSpec == nil && req.Manifest.Shift != nil {
		// Fallback to deprecated Shift for backward compatibility.
		gateSpec = &contracts.StepGateSpec{
			Enabled: req.Manifest.Shift.Enabled, //lint:ignore SA1019 compat field access
			Profile: req.Manifest.Shift.Profile, //lint:ignore SA1019 compat field access
			Env:     req.Manifest.Shift.Env,     //lint:ignore SA1019 compat field access
		}
	}
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
			result.Timings.BuildGateDuration = time.Since(gateStart)
			result.Timings.TotalDuration = time.Since(totalStart)
			return result, fmt.Errorf("%w: %s", ErrBuildGateFailed, "pre-mod validation failed")
		}
	}
	result.Timings.BuildGateDuration = time.Since(gateStart)

	// Stage 3: Execute container via configured runtime (real execution when available).
	executionStart := time.Now()
	if r.Containers == nil {
		// Backward-compatible fallback: simulate execution when no runtime is configured
		if r.LogWriter != nil {
			_, _ = fmt.Fprintf(r.LogWriter, "Starting execution for manifest %s\n", req.Manifest.ID)
		}
		result.ExitCode = 0
		result.Timings.ExecutionDuration = time.Since(executionStart)
	} else {
		// Build container spec from manifest and workspace path plus optional /out and /in mounts.
		spec, err := buildContainerSpec(req.TicketID, req.Manifest, req.Workspace, req.OutDir, req.InDir)
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
		result.Timings.ExecutionDuration = time.Since(executionStart)

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
	result.Timings.DiffDuration = time.Since(diffStart)

	// Stage 5: (removed) Artifact publishing is performed by the node agent.

	result.Timings.TotalDuration = time.Since(totalStart)
	return result, nil
}
