package step

import (
	"context"
	"errors"

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

// FilesystemWorkspaceHydratorOptions holds configuration for workspace hydrator.
type FilesystemWorkspaceHydratorOptions struct {
	RepoFetcher interface{}
}

// WorkspaceHydrator prepares a workspace for execution.
type WorkspaceHydrator interface{}

type filesystemWorkspaceHydrator struct{}

// NewFilesystemWorkspaceHydrator creates a new workspace hydrator.
func NewFilesystemWorkspaceHydrator(opts FilesystemWorkspaceHydratorOptions) (WorkspaceHydrator, error) {
	_ = opts
	return &filesystemWorkspaceHydrator{}, nil
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
type DiffGenerator interface{}

type filesystemDiffGenerator struct{}

// NewFilesystemDiffGenerator creates a new diff generator.
func NewFilesystemDiffGenerator(opts FilesystemDiffGeneratorOptions) DiffGenerator {
	_ = opts
	return &filesystemDiffGenerator{}
}

// Runner executes workflow steps.
type Runner struct {
	Workspace  WorkspaceHydrator
	Containers ContainerRuntime
	Diffs      DiffGenerator
	Artifacts  ArtifactPublisher
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
}

// Run executes a step and returns the result.
func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	_ = ctx
	_ = req
	return Result{}, errors.New("not implemented")
}
