package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

func TestLocalStepClientInvokesRunnerAndSurfacesShiftFailure(t *testing.T) {
	manifest := contracts.StepManifest{
		ID:    "mods-plan",
		Name:  "Mods Plan",
		Image: "ghcr.io/ploy/mods-plan:latest",
		Command: []string{
			"/bin/run",
		},
		Inputs: []contracts.StepInput{
			{Name: "baseline", MountPath: "/mnt/baseline", Mode: contracts.StepInputModeReadOnly, SnapshotCID: "bafy-baseline"},
			{Name: "overlay", MountPath: "/mnt/overlay", Mode: contracts.StepInputModeReadWrite, DiffCID: "bafy-overlay"},
		},
		Shift: &contracts.StepShiftSpec{Enabled: true, Profile: "default"},
	}
	fake := &recordingStepRunner{
		result: step.Result{
			ExitCode: 1,
			DiffArtifact: step.PublishedArtifact{
				CID:  "bafy-diff",
				Kind: step.ArtifactKindDiff,
			},
			LogArtifact: step.PublishedArtifact{
				CID:  "bafy-logs",
				Kind: step.ArtifactKindLogs,
			},
			ShiftReport: step.ShiftResult{
				Passed:  false,
				Message: "build gate failed: go vet reported errors",
				Report:  []byte(`{"static_checks":[{"tool":"go vet"}]}`),
			},
			Retained:     true,
			RetentionTTL: "24h",
		},
	}
	client, err := NewLocalStepClient(LocalStepClientOptions{Runner: fake})
	if err != nil {
		t.Fatalf("NewLocalStepClient() error: %v", err)
	}
	stage := runner.Stage{
		Name:         "build-gate",
		Kind:         runner.StageKindBuildGate,
		Lane:         "build-gate",
		StepManifest: &manifest,
	}
	ticket := contracts.WorkflowTicket{TicketID: "ticket-123", Tenant: "acme"}
	outcome, err := client.ExecuteStage(context.Background(), ticket, stage, "/tmp/workspace")
	if err != nil {
		t.Fatalf("ExecuteStage() unexpected error: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("expected step runner invoked once, got %d", fake.calls)
	}
	if fake.last.Manifest.ID != manifest.ID {
		t.Fatalf("expected manifest passed through, got %q", fake.last.Manifest.ID)
	}
	if fake.last.Workspace != "/tmp/workspace" {
		t.Fatalf("expected workspace forwarded, got %q", fake.last.Workspace)
	}
	if outcome.Status != runner.StageStatusFailed {
		t.Fatalf("expected stage to fail, got %s", outcome.Status)
	}
	if !strings.Contains(outcome.Message, "go vet") {
		t.Fatalf("expected shift diagnostics propagated, got %q", outcome.Message)
	}
	if len(outcome.Artifacts) != 2 {
		t.Fatalf("expected diff and log artifacts, got %d", len(outcome.Artifacts))
	}
	if !containsArtifact(outcome.Artifacts, "diff", "bafy-diff") {
		t.Fatalf("expected diff artifact in outcome, got %+v", outcome.Artifacts)
	}
	if !containsArtifact(outcome.Artifacts, "logs", "bafy-logs") {
		t.Fatalf("expected log artifact in outcome, got %+v", outcome.Artifacts)
	}
}

type recordingStepRunner struct {
	result step.Result
	err    error
	calls  int
	last   step.Request
}

func (r *recordingStepRunner) Run(ctx context.Context, req step.Request) (step.Result, error) {
	_ = ctx
	r.calls++
	r.last = req
	return r.result, r.err
}

func containsArtifact(artifacts []runner.Artifact, name, cid string) bool {
	for _, artifact := range artifacts {
		if artifact.Name == name && artifact.ArtifactCID == cid {
			return true
		}
	}
	return false
}
