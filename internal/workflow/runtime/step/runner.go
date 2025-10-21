package step

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// ErrManifestInvalid indicates the provided manifest failed validation.
var ErrManifestInvalid = errors.New("step: manifest invalid")

// ErrWorkspaceUnavailable indicates workspace hydration failed.
var ErrWorkspaceUnavailable = errors.New("step: workspace unavailable")

// ErrShiftFailed indicates SHIFT validation failed.
var ErrShiftFailed = errors.New("step: SHIFT validation failed")

// Request captures the data required to execute a step manifest.
type Request struct {
	Manifest  contracts.StepManifest
	Workspace string
}

// Result summarises a completed step run.
type Result struct {
	ContainerID  string
	ExitCode     int
	DiffArtifact PublishedArtifact
	LogArtifact  PublishedArtifact
	ShiftReport  ShiftResult
	RetentionTTL string
	Retained     bool
}

// Runner executes step manifests using the injected collaborators.
type Runner struct {
	Workspace  WorkspaceHydrator
	Containers ContainerRuntime
	Diffs      DiffGenerator
	SHIFT      ShiftClient
	Artifacts  ArtifactPublisher
	Logs       LogCollector
}

// Run executes the step manifest and returns the execution result.
func (r Runner) Run(ctx context.Context, req Request) (Result, error) {
	manifest := req.Manifest
	if err := manifest.Validate(); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if r.Workspace == nil {
		return Result{}, fmt.Errorf("%w: workspace hydrator missing", ErrWorkspaceUnavailable)
	}
	workspace, err := r.Workspace.Prepare(ctx, WorkspaceRequest{Manifest: manifest})
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrWorkspaceUnavailable, err)
	}

	spec, err := buildContainerSpec(manifest, workspace)
	if err != nil {
		return Result{}, err
	}

	if r.Containers == nil {
		return Result{}, errors.New("step: container runtime missing")
	}
	handle, err := r.Containers.Create(ctx, spec)
	if err != nil {
		return Result{}, fmt.Errorf("step: create container: %w", err)
	}
	if err := r.Containers.Start(ctx, handle); err != nil {
		return Result{}, fmt.Errorf("step: start container: %w", err)
	}
	containerResult, err := r.Containers.Wait(ctx, handle)
	if err != nil {
		return Result{}, fmt.Errorf("step: wait container: %w", err)
	}

	var logBytes []byte
	if r.Logs != nil {
		logBytes, err = r.Logs.Collect(ctx, handle)
	} else {
		logBytes, err = r.Containers.Logs(ctx, handle)
	}
	if err != nil {
		return Result{}, fmt.Errorf("step: collect logs: %w", err)
	}

	if r.Diffs == nil {
		return Result{}, errors.New("step: diff generator missing")
	}
	diffResult, err := r.Diffs.Capture(ctx, DiffRequest{Manifest: manifest, Workspace: workspace})
	if err != nil {
		return Result{}, fmt.Errorf("step: capture diff: %w", err)
	}

	var diffArtifact PublishedArtifact
	var logArtifact PublishedArtifact
	if r.Artifacts != nil {
		diffArtifact, err = r.Artifacts.Publish(ctx, ArtifactRequest{Kind: ArtifactKindDiff, Path: diffResult.Path})
		if err != nil {
			return Result{}, fmt.Errorf("step: publish diff: %w", err)
		}
		logArtifact, err = r.Artifacts.Publish(ctx, ArtifactRequest{Kind: ArtifactKindLogs, Buffer: logBytes})
		if err != nil {
			return Result{}, fmt.Errorf("step: publish logs: %w", err)
		}
	}

	shiftResult := ShiftResult{Passed: true}
	if r.SHIFT != nil && manifest.Shift != nil {
		shiftReq := ShiftRequest{
			Manifest:  manifest,
			Workspace: workspace,
		}
		if logArtifact.CID != "" {
			artifactCopy := logArtifact
			shiftReq.LogArtifact = &artifactCopy
		}
		shiftResult, err = r.SHIFT.Validate(ctx, shiftReq)
		if err != nil {
			return Result{}, fmt.Errorf("step: SHIFT validation: %w", err)
		}
		if !shiftResult.Passed {
			return Result{
				ContainerID:  handle.ID,
				ExitCode:     containerResult.ExitCode,
				DiffArtifact: diffArtifact,
				LogArtifact:  logArtifact,
				ShiftReport:  shiftResult,
				Retained:     manifest.Retention.RetainContainer,
				RetentionTTL: manifest.Retention.TTL,
			}, fmt.Errorf("%w: %s", ErrShiftFailed, shiftResult.Message)
		}
	}

	return Result{
		ContainerID:  handle.ID,
		ExitCode:     containerResult.ExitCode,
		DiffArtifact: diffArtifact,
		LogArtifact:  logArtifact,
		ShiftReport:  shiftResult,
		Retained:     manifest.Retention.RetainContainer,
		RetentionTTL: manifest.Retention.TTL,
	}, nil
}

func buildContainerSpec(manifest contracts.StepManifest, workspace Workspace) (ContainerSpec, error) {
	mounts := make([]ContainerMount, 0, len(manifest.Inputs))
	for _, input := range manifest.Inputs {
		path, ok := workspace.Inputs[input.Name]
		if !ok {
			return ContainerSpec{}, fmt.Errorf("step: workspace missing input %q", input.Name)
		}
		mounts = append(mounts, ContainerMount{
			Source:   path,
			Target:   input.MountPath,
			ReadOnly: input.Mode == contracts.StepInputModeReadOnly,
		})
	}
	command := append([]string{}, manifest.Command...)
	if len(manifest.Args) > 0 {
		command = append(command, manifest.Args...)
	}
	env := make(map[string]string, len(manifest.Env))
	if len(manifest.Env) > 0 {
		keys := make([]string, 0, len(manifest.Env))
		for key := range manifest.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			env[key] = manifest.Env[key]
		}
	}
	workingDir := manifest.WorkingDir
	if strings.TrimSpace(workingDir) == "" {
		workingDir = workspace.WorkingDir
	}
	return ContainerSpec{
		Image:      manifest.Image,
		Command:    command,
		WorkingDir: workingDir,
		Env:        env,
		Mounts:     mounts,
		Retain:     manifest.Retention.RetainContainer,
	}, nil
}

// ContainerRuntime executes containers for step runs.
type ContainerRuntime interface {
	Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error)
	Start(ctx context.Context, handle ContainerHandle) error
	Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error)
	Logs(ctx context.Context, handle ContainerHandle) ([]byte, error)
}

// ContainerSpec describes a container execution request.
type ContainerSpec struct {
	Image      string
	Command    []string
	WorkingDir string
	Env        map[string]string
	Mounts     []ContainerMount
	Retain     bool
}

// ContainerMount describes a host path mount.
type ContainerMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// ContainerHandle identifies a prepared container.
type ContainerHandle struct {
	ID string
}

// ContainerResult captures container exit metadata.
type ContainerResult struct {
	ExitCode    int
	StartedAt   time.Time
	CompletedAt time.Time
}

// WorkspaceHydrator prepares the workspace for execution.
type WorkspaceHydrator interface {
	Prepare(ctx context.Context, req WorkspaceRequest) (Workspace, error)
}

// WorkspaceRequest asks hydrator to materialise inputs.
type WorkspaceRequest struct {
	Manifest contracts.StepManifest
}

// Workspace describes hydrated paths.
type Workspace struct {
	Inputs     map[string]string
	WorkingDir string
}

// DiffGenerator captures diffs after execution.
type DiffGenerator interface {
	Capture(ctx context.Context, req DiffRequest) (DiffResult, error)
}

// DiffRequest contains diff capture metadata.
type DiffRequest struct {
	Manifest  contracts.StepManifest
	Workspace Workspace
}

// DiffResult summarises the captured diff artifact.
type DiffResult struct {
	Path string
}

// ShiftClient invokes the SHIFT build gate.
type ShiftClient interface {
	Validate(ctx context.Context, req ShiftRequest) (ShiftResult, error)
}

// ShiftRequest wraps manifest + workspace context.
type ShiftRequest struct {
	Manifest    contracts.StepManifest
	Workspace   Workspace
	LogArtifact *PublishedArtifact
}

// ShiftResult contains SHIFT execution details.
type ShiftResult struct {
	Passed  bool
	Message string
	Report  []byte
}

// ArtifactPublisher uploads step artifacts.
type ArtifactPublisher interface {
	Publish(ctx context.Context, req ArtifactRequest) (PublishedArtifact, error)
}

// ArtifactRequest describes an artifact to publish.
type ArtifactRequest struct {
	Kind   ArtifactKind
	Path   string
	Buffer []byte
}

// ArtifactKind enumerates artifact types.
type ArtifactKind string

const (
	// ArtifactKindDiff identifies diff bundles.
	ArtifactKindDiff ArtifactKind = "diff"
	// ArtifactKindLogs identifies log bundles.
	ArtifactKindLogs ArtifactKind = "logs"
)

// PublishedArtifact references a stored artifact.
type PublishedArtifact struct {
	CID  string
	Kind ArtifactKind
}

// LogCollector retrieves container logs when a custom pathway exists.
type LogCollector interface {
	Collect(ctx context.Context, handle ContainerHandle) ([]byte, error)
}
