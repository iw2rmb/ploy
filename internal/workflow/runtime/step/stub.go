package step

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Stub implementations for workflow runtime step package.
// These are minimal placeholders to allow compilation until full implementation.

// Artifact kind constants.
const (
	ArtifactKindLogs = "logs"
	ArtifactKindDiff = "diff"
)

// FilesystemArtifactPublisherOptions holds configuration for the filesystem artifact publisher.
type FilesystemArtifactPublisherOptions struct{}

// ArtifactPublisher publishes build artifacts.
type ArtifactPublisher interface {
	Publish(ctx context.Context, req ArtifactRequest) (PublishedArtifact, error)
}

// ArtifactRequest describes an artifact to publish.
type ArtifactRequest struct {
	Kind   string
	Path   string
	Buffer []byte
}

// PublishedArtifact represents a successfully published artifact.
type PublishedArtifact struct {
	CID    string
	Kind   string
	Digest string
	Size   int64
}

type filesystemArtifactPublisher struct{}

// NewFilesystemArtifactPublisher creates a new filesystem artifact publisher.
func NewFilesystemArtifactPublisher(opts FilesystemArtifactPublisherOptions) (ArtifactPublisher, error) {
	_ = opts
	return &filesystemArtifactPublisher{}, nil
}

func (p *filesystemArtifactPublisher) Publish(ctx context.Context, req ArtifactRequest) (PublishedArtifact, error) {
	_ = ctx
	_ = req
	return PublishedArtifact{}, errors.New("not implemented")
}

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
	PullImage bool
}

// ContainerRuntime executes containers.
type ContainerRuntime interface{}

type dockerContainerRuntime struct{}

// NewDockerContainerRuntime creates a new Docker container runtime.
func NewDockerContainerRuntime(opts DockerContainerRuntimeOptions) (ContainerRuntime, error) {
	_ = opts
	return &dockerContainerRuntime{}, nil
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
		// If stderr has content, there was likely a real error.
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("git diff failed: %s", stderr.String())
		}
	}

	return stdout.Bytes(), nil
}

// Runner executes workflow steps.
type Runner struct {
	Workspace  WorkspaceHydrator
	Containers ContainerRuntime
	Diffs      DiffGenerator
	Artifacts  ArtifactPublisher
	Gate       GateExecutor
	LogWriter  io.Writer // Optional: streams logs to server as gzipped chunks.
}

// Request describes a step execution request.
type Request struct {
	Manifest  contracts.StepManifest
	Workspace string
}

// Result contains the outcome of a step execution.
type Result struct {
	ExitCode     int
	DiffArtifact PublishedArtifact
	LogArtifact  PublishedArtifact
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

	// Stage 2: Execute container (placeholder for now).
	executionStart := time.Now()
	// Container execution is stubbed; future work will invoke Containers.
	// For now, we simulate successful execution with exit code 0.
	// When container execution is implemented, stdout/stderr will be written to LogWriter.
	if r.LogWriter != nil {
		// Placeholder log message to demonstrate streaming works.
		_, _ = fmt.Fprintf(r.LogWriter, "Starting execution for manifest %s\n", req.Manifest.ID)
		_, _ = fmt.Fprintf(r.LogWriter, "Workspace: %s\n", req.Workspace)
		_, _ = fmt.Fprintf(r.LogWriter, "Image: %s\n", req.Manifest.Image)
		_, _ = fmt.Fprintf(r.LogWriter, "Command: %v\n", req.Manifest.Command)
	}
	result.ExitCode = 0
	result.Timings.ExecutionDuration = time.Since(executionStart)

	// Stage 3: Build gate validation.
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
	}
	result.Timings.BuildGateDuration = time.Since(gateStart)

	// Stage 4: Generate diff.
	diffStart := time.Now()
	if r.Diffs != nil {
		diffBytes, err := r.Diffs.Generate(ctx, req.Workspace)
		if err != nil {
			return Result{}, fmt.Errorf("diff generation failed: %w", err)
		}
		// Publish diff as an artifact if there are changes.
		if len(diffBytes) > 0 && r.Artifacts != nil {
			diffArtifact, err := r.Artifacts.Publish(ctx, ArtifactRequest{
				Kind:   ArtifactKindDiff,
				Buffer: diffBytes,
			})
			if err != nil {
				return Result{}, fmt.Errorf("diff publish failed: %w", err)
			}
			result.DiffArtifact = diffArtifact
		}
	}
	result.Timings.DiffDuration = time.Since(diffStart)

	// Stage 5: Publish artifacts (placeholder for now).
	publishStart := time.Now()
	// Artifact publishing is stubbed; future work will invoke Artifacts.
	result.Timings.PublishDuration = time.Since(publishStart)

	result.Timings.TotalDuration = time.Since(totalStart)
	return result, nil
}
