package step

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestRunnerBuildsContainerSpec(t *testing.T) {
	ctx := context.Background()
	manifest := contracts.StepManifest{
		ID:         "mods-orw-apply",
		Name:       "ORW Apply",
		Image:      "ghcr.io/ploy/mods/openrewrite:latest",
		Command:    []string{"/bin/run"},
		Args:       []string{"--execute"},
		WorkingDir: "/workspace",
		Env: map[string]string{
			"JAVA_TOOL_OPTIONS": "-Xmx2g",
		},
		Inputs: []contracts.StepInput{
			{Name: "baseline", MountPath: "/workspace", Mode: contracts.StepInputModeReadOnly, SnapshotCID: "bafybaseline"},
			{Name: "overlay", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite, DiffCID: "bafyoverlay"},
		},
		Shift:     &contracts.StepShiftSpec{Profile: "default"},
		Retention: contracts.StepRetentionSpec{RetainContainer: true, TTL: "24h"},
	}

	container := &fakeContainerRunner{}
	workspace := &fakeWorkspaceHydrator{
		inputs: map[string]string{
			"baseline": "/tmp/workspace/base",
			"overlay":  "/tmp/workspace/overlay",
		},
	}
	diffs := &fakeDiffGenerator{result: DiffResult{Path: "/tmp/diff"}}
	shift := &fakeShiftClient{result: ShiftResult{Passed: true}}
	artifacts := &fakeArtifactPublisher{}
	logger := &fakeLogCollector{logs: []byte("log output")}

	r := Runner{
		Workspace:  workspace,
		Containers: container,
		Diffs:      diffs,
		SHIFT:      shift,
		Artifacts:  artifacts,
		Logs:       logger,
	}

	result, err := r.Run(ctx, Request{Manifest: manifest})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.ContainerID != container.handle.ID {
		t.Fatalf("expected container id %q, got %q", container.handle.ID, result.ContainerID)
	}
	if !container.started {
		t.Fatalf("container was not started")
	}
	if container.spec.Image != manifest.Image {
		t.Fatalf("container image mismatch: want %q got %q", manifest.Image, container.spec.Image)
	}
	if got := strings.Join(container.spec.Command, " "); got != "/bin/run --execute" {
		t.Fatalf("container command mismatch: %q", got)
	}
	if container.spec.WorkingDir != manifest.WorkingDir {
		t.Fatalf("working dir mismatch: %q", container.spec.WorkingDir)
	}
	if len(container.spec.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(container.spec.Mounts))
	}
	if !container.spec.Mounts[0].ReadOnly {
		t.Fatalf("baseline mount should be read-only")
	}
	if container.spec.Mounts[1].ReadOnly {
		t.Fatalf("overlay mount should be read-write")
	}
	if container.spec.Env["JAVA_TOOL_OPTIONS"] != "-Xmx2g" {
		t.Fatalf("env not forwarded")
	}
	if !logger.collected {
		t.Fatalf("expected logs to be collected")
	}
	if len(artifacts.published) == 0 {
		t.Fatalf("expected artifacts to be published")
	}
}

func TestRunnerShiftFailureBlocksPipeline(t *testing.T) {
	ctx := context.Background()
	manifest := contracts.StepManifest{
		ID:    "mods-plan",
		Name:  "Mods Plan",
		Image: "ghcr.io/ploy/mods/plan:latest",
		Inputs: []contracts.StepInput{
			{Name: "baseline", MountPath: "/workspace", Mode: contracts.StepInputModeReadOnly, SnapshotCID: "bafy"},
			{Name: "overlay", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite, DiffCID: "bafy2"},
		},
		Shift: &contracts.StepShiftSpec{Profile: "default"},
	}

	container := &fakeContainerRunner{}
	workspace := &fakeWorkspaceHydrator{
		inputs: map[string]string{
			"baseline": "/tmp/workspace/base",
			"overlay":  "/tmp/workspace/overlay",
		},
	}
	diffs := &fakeDiffGenerator{result: DiffResult{Path: "/tmp/diff"}}
	shift := &fakeShiftClient{result: ShiftResult{Passed: false, Message: "tests failed"}}
	artifacts := &fakeArtifactPublisher{}
	logger := &fakeLogCollector{logs: []byte("log output")}

	r := Runner{
		Workspace:  workspace,
		Containers: container,
		Diffs:      diffs,
		SHIFT:      shift,
		Artifacts:  artifacts,
		Logs:       logger,
	}

	_, err := r.Run(ctx, Request{Manifest: manifest})
	if err == nil {
		t.Fatalf("expected error from SHIFT failure")
	}
	if !strings.Contains(err.Error(), "SHIFT") {
		t.Fatalf("expected error to mention SHIFT, got %v", err)
	}
	if len(artifacts.published) == 0 {
		t.Fatalf("expected artifact publication even on failure")
	}
	if artifacts.published[0].Kind != ArtifactKindDiff {
		t.Fatalf("expected diff artifact to publish")
	}
}

type fakeContainerRunner struct {
	spec    ContainerSpec
	handle  ContainerHandle
	started bool
}

func (f *fakeContainerRunner) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	f.spec = spec
	f.handle = ContainerHandle{ID: "container-123"}
	return f.handle, nil
}

func (f *fakeContainerRunner) Start(ctx context.Context, handle ContainerHandle) error {
	if handle.ID == "" {
		return errors.New("missing container id")
	}
	f.started = true
	return nil
}

func (f *fakeContainerRunner) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	return ContainerResult{ExitCode: 0, StartedAt: time.Now().Add(-1 * time.Second), CompletedAt: time.Now()}, nil
}

func (f *fakeContainerRunner) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	return []byte("log output"), nil
}

type fakeWorkspaceHydrator struct {
	inputs map[string]string
}

func (f *fakeWorkspaceHydrator) Prepare(ctx context.Context, req WorkspaceRequest) (Workspace, error) {
	return Workspace{Inputs: f.inputs, WorkingDir: "/tmp/workspace"}, nil
}

type fakeDiffGenerator struct {
	result DiffResult
}

func (f *fakeDiffGenerator) Capture(ctx context.Context, req DiffRequest) (DiffResult, error) {
	return f.result, nil
}

type fakeShiftClient struct {
	result ShiftResult
}

func (f *fakeShiftClient) Validate(ctx context.Context, req ShiftRequest) (ShiftResult, error) {
	return f.result, nil
}

type fakeArtifactPublisher struct {
	published []PublishedArtifact
}

func (f *fakeArtifactPublisher) Publish(ctx context.Context, req ArtifactRequest) (PublishedArtifact, error) {
	artifact := PublishedArtifact{CID: "bafydiff", Kind: req.Kind}
	f.published = append(f.published, artifact)
	return artifact, nil
}

type fakeLogCollector struct {
	logs      []byte
	collected bool
}

func (f *fakeLogCollector) Collect(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	f.collected = true
	return bytes.Clone(f.logs), nil
}
