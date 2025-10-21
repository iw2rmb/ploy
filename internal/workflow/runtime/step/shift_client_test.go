package step

import (
	"context"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestBuildGateShiftClientReportsFailures(t *testing.T) {
	runner := &fakeBuildGateRunner{
		result: buildgate.RunResult{
			Sandbox: buildgate.SandboxOutcome{
				Status:        buildgate.SandboxStatusFailed,
				FailureReason: "tests",
				FailureDetail: "go test ./... failed",
				LogDigest:     "bafy-logs",
			},
			StaticChecks: []buildgate.StaticCheckReport{
				{
					Language: "go",
					Tool:     "go vet",
					Passed:   false,
					Failures: []buildgate.StaticCheckFailure{{
						RuleID:   "GOVET001",
						File:     "./main.go",
						Line:     42,
						Column:   7,
						Severity: "error",
						Message:  "unused import",
					}},
				},
			},
			Metadata: buildgate.Metadata{
				LogDigest: "bafy-logs",
				StaticChecks: []buildgate.StaticCheckReport{
					{
						Language: "go",
						Tool:     "go vet",
						Passed:   false,
						Failures: []buildgate.StaticCheckFailure{{
							RuleID:   "GOVET001",
							File:     "./main.go",
							Line:     42,
							Column:   7,
							Severity: "error",
							Message:  "unused import",
						}},
					},
				},
			},
		},
	}
	client, err := NewBuildGateShiftClient(BuildGateShiftOptions{Runner: runner})
	if err != nil {
		t.Fatalf("NewBuildGateShiftClient() error: %v", err)
	}

	manifest := contracts.StepManifest{
		ID:      "mods-plan",
		Name:    "Mods Plan",
		Image:   "ghcr.io/ploy/mods-plan:latest",
		Command: []string{"/bin/run"},
		Inputs: []contracts.StepInput{
			{Name: "baseline", MountPath: "/mnt/baseline", Mode: contracts.StepInputModeReadOnly, SnapshotCID: "bafy-baseline"},
			{Name: "overlay", MountPath: "/mnt/overlay", Mode: contracts.StepInputModeReadWrite, DiffCID: "bafy-overlay"},
		},
		Shift: &contracts.StepShiftSpec{Enabled: true, Profile: "default"},
	}

	logArtifact := PublishedArtifact{CID: "bafy-logs", Kind: ArtifactKindLogs, Digest: "sha256:fixture"}
	result, err := client.Validate(context.Background(), ShiftRequest{
		Manifest:    manifest,
		Workspace:   Workspace{WorkingDir: "/tmp/workspace"},
		LogArtifact: &logArtifact,
	})
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
	if result.Passed {
		t.Fatalf("expected shift failure")
	}
	if !strings.Contains(result.Message, "go vet") || !strings.Contains(result.Message, "unused import") {
		t.Fatalf("expected failure diagnostics in message, got %q", result.Message)
	}
	if len(result.Report) == 0 {
		t.Fatalf("expected serialized report in ShiftResult")
	}
	if len(runner.specs) != 1 {
		t.Fatalf("expected runner invoked once, got %d", len(runner.specs))
	}
	if runner.specs[0].LogArtifact == nil {
		t.Fatalf("expected log artifact to be requested in run spec")
	}
}

func TestBuildGateShiftClientPropagatesRunnerError(t *testing.T) {
	runner := &fakeBuildGateRunner{err: context.DeadlineExceeded}
	client, err := NewBuildGateShiftClient(BuildGateShiftOptions{Runner: runner})
	if err != nil {
		t.Fatalf("NewBuildGateShiftClient() error: %v", err)
	}
	manifest := contracts.StepManifest{
		ID:      "mods-plan",
		Name:    "Mods Plan",
		Image:   "ghcr.io/ploy/mods-plan:latest",
		Command: []string{"/bin/run"},
		Inputs: []contracts.StepInput{
			{Name: "baseline", MountPath: "/mnt/baseline", Mode: contracts.StepInputModeReadOnly, SnapshotCID: "bafy-baseline"},
			{Name: "overlay", MountPath: "/mnt/overlay", Mode: contracts.StepInputModeReadWrite, DiffCID: "bafy-overlay"},
		},
		Shift: &contracts.StepShiftSpec{Enabled: true, Profile: "default"},
	}
	_, err = client.Validate(context.Background(), ShiftRequest{
		Manifest:  manifest,
		Workspace: Workspace{WorkingDir: "/tmp/workspace"},
	})
	if err == nil {
		t.Fatalf("expected error when runner fails")
	}
}

type fakeBuildGateRunner struct {
	result buildgate.RunResult
	err    error
	specs  []buildgate.RunSpec
}

func (f *fakeBuildGateRunner) Run(ctx context.Context, spec buildgate.RunSpec) (buildgate.RunResult, error) {
	_ = ctx
	f.specs = append(f.specs, spec)
	return f.result, f.err
}
