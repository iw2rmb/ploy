//go:build integration

package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

func TestIntegrationStepRunnerCapturesDiffAndShift(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dir := t.TempDir()
	baselineDir := filepath.Join(dir, "baseline")
	overlayDir := filepath.Join(dir, "overlay")
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		t.Fatalf("mkdir baseline: %v", err)
	}
	if err := os.MkdirAll(overlayDir, 0o755); err != nil {
		t.Fatalf("mkdir overlay: %v", err)
	}
	baseFile := filepath.Join(baselineDir, "hello.txt")
	if err := os.WriteFile(baseFile, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	overlayFile := filepath.Join(overlayDir, "hello.txt")
	if err := os.WriteFile(overlayFile, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	manifest := contracts.StepManifest{
		ID:    "mods-sample",
		Name:  "Sample",
		Image: "ghcr.io/ploy/runtime/sample:latest",
		Inputs: []contracts.StepInput{
			{Name: "baseline", MountPath: "/workspace", Mode: contracts.StepInputModeReadOnly, SnapshotCID: "bafybaseline"},
			{Name: "overlay", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite, DiffCID: "bafyoverlay"},
		},
		Shift: &contracts.StepShiftSpec{Profile: "default"},
	}

	runtime := &integrationRuntime{overlay: overlayFile}
	workspace := &integrationWorkspaceHydrator{paths: map[string]string{"baseline": baselineDir, "overlay": overlayDir}, workingDir: overlayDir}
	diffs := &fileDiffGenerator{}
	shift := &integrationShiftClient{}
	artifacts := &integrationArtifactPublisher{}
	logs := &integrationLogCollector{}

	r := step.Runner{
		Workspace:  workspace,
		Containers: runtime,
		Diffs:      diffs,
		SHIFT:      shift,
		Artifacts:  artifacts,
		Logs:       logs,
	}

	res, err := r.Run(ctx, step.Request{Manifest: manifest})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}
	if !shift.called {
		t.Fatalf("expected SHIFT to be invoked")
	}
	if len(artifacts.published) != 2 {
		t.Fatalf("expected diff and log artifacts, got %d", len(artifacts.published))
	}
	diffArtifact := artifacts.published[0]
	if diffArtifact.Kind != step.ArtifactKindDiff {
		t.Fatalf("expected diff artifact kind, got %s", diffArtifact.Kind)
	}
	if !strings.Contains(string(diffs.lastContent), "+hello world v2") {
		t.Fatalf("diff output missing expected change: %s", string(diffs.lastContent))
	}
	if !strings.Contains(string(artifacts.diffContent), "+hello world v2") {
		t.Fatalf("artifact diff missing change: %s", string(artifacts.diffContent))
	}
}

type integrationRuntime struct {
	spec    step.ContainerSpec
	handle  step.ContainerHandle
	overlay string
}

func (i *integrationRuntime) Create(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
	i.spec = spec
	i.handle = step.ContainerHandle{ID: "integration-container"}
	return i.handle, nil
}

func (i *integrationRuntime) Start(ctx context.Context, handle step.ContainerHandle) error {
	return os.WriteFile(i.overlay, []byte("hello world v2\n"), 0o644)
}

func (i *integrationRuntime) Wait(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
	return step.ContainerResult{ExitCode: 0, StartedAt: time.Now(), CompletedAt: time.Now()}, nil
}

func (i *integrationRuntime) Logs(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
	return []byte("integration logs"), nil
}

type integrationWorkspaceHydrator struct {
	paths      map[string]string
	workingDir string
}

func (i integrationWorkspaceHydrator) Prepare(ctx context.Context, req step.WorkspaceRequest) (step.Workspace, error) {
	return step.Workspace{Inputs: i.paths, WorkingDir: i.workingDir}, nil
}

type fileDiffGenerator struct {
	lastContent []byte
}

func (f *fileDiffGenerator) Capture(ctx context.Context, req step.DiffRequest) (step.DiffResult, error) {
	baselinePath := req.Workspace.Inputs["baseline"]
	overlayPath := req.Workspace.Inputs["overlay"]
	baselineData, err := os.ReadFile(filepath.Join(baselinePath, "hello.txt"))
	if err != nil {
		return step.DiffResult{}, err
	}
	overlayData, err := os.ReadFile(filepath.Join(overlayPath, "hello.txt"))
	if err != nil {
		return step.DiffResult{}, err
	}
	diff := []byte("-" + strings.TrimSpace(string(baselineData)) + "\n+" + strings.TrimSpace(string(overlayData)) + "\n")
	f.lastContent = diff
	diffPath := filepath.Join(overlayPath, "diff.patch")
	if err := os.WriteFile(diffPath, diff, 0o644); err != nil {
		return step.DiffResult{}, err
	}
	return step.DiffResult{Path: diffPath}, nil
}

type integrationShiftClient struct {
	called bool
}

func (i *integrationShiftClient) Validate(ctx context.Context, req step.ShiftRequest) (step.ShiftResult, error) {
	i.called = true
	return step.ShiftResult{Passed: true}, nil
}

type integrationArtifactPublisher struct {
	published   []step.PublishedArtifact
	diffContent []byte
}

func (i *integrationArtifactPublisher) Publish(ctx context.Context, req step.ArtifactRequest) (step.PublishedArtifact, error) {
	artifact := step.PublishedArtifact{CID: string(req.Kind) + "-cid", Kind: req.Kind}
	if req.Kind == step.ArtifactKindDiff {
		content, err := os.ReadFile(req.Path)
		if err != nil {
			return step.PublishedArtifact{}, err
		}
		i.diffContent = content
	}
	i.published = append(i.published, artifact)
	return artifact, nil
}

type integrationLogCollector struct{}

func (integrationLogCollector) Collect(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
	return []byte("integration logs"), nil
}
