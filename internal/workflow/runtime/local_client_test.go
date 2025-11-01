package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

func TestLocalStepClientInvokesRunnerAndSurfacesGateFailure(t *testing.T) {
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
				CID:    "bafy-diff",
				Kind:   step.ArtifactKindDiff,
				Digest: "sha256:d00d",
			},
			LogArtifact: step.PublishedArtifact{
				CID:    "bafy-logs",
				Kind:   step.ArtifactKindLogs,
				Digest: "sha256:beef",
			},
			GateArtifact: step.PublishedArtifact{
				CID:    "bafy-gate",
				Kind:   step.ArtifactKindGateReport,
				Digest: "sha256:feed",
			},
			GateReport: step.GateResult{
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
	ticket := contracts.WorkflowTicket{TicketID: "ticket-123"}
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
		t.Fatalf("expected build gate diagnostics propagated, got %q", outcome.Message)
	}
	if len(outcome.Artifacts) != 3 {
		t.Fatalf("expected diff, log, and gate artifacts, got %d", len(outcome.Artifacts))
	}
	if !containsArtifact(outcome.Artifacts, "diff", "bafy-diff") {
		t.Fatalf("expected diff artifact in outcome, got %+v", outcome.Artifacts)
	}
	if !containsArtifact(outcome.Artifacts, "logs", "bafy-logs") {
		t.Fatalf("expected log artifact in outcome, got %+v", outcome.Artifacts)
	}
	if !containsArtifact(outcome.Artifacts, "gate_report", "bafy-gate") {
		t.Fatalf("expected gate artifact in outcome, got %+v", outcome.Artifacts)
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

func TestLocalStepClientRecordsStageInvocation(t *testing.T) {
	manifest := contracts.StepManifest{
		ID:    "mods-apply",
		Name:  "Mods Apply",
		Image: "ghcr.io/ploy/mods-apply:latest",
		Inputs: []contracts.StepInput{
			{Name: "baseline", MountPath: "/mnt/baseline", Mode: contracts.StepInputModeReadOnly, SnapshotCID: "bafy-1"},
			{Name: "overlay", MountPath: "/mnt/overlay", Mode: contracts.StepInputModeReadWrite, DiffCID: "bafy-2"},
		},
		Retention: contracts.StepRetentionSpec{RetainContainer: true, TTL: "36h"},
	}
	stepRunner := &recordingStepRunner{
		result: step.Result{
			ContainerID: "container-789",
			ExitCode:    0,
			DiffArtifact: step.PublishedArtifact{
				CID:    "bafy-diff-apply",
				Kind:   step.ArtifactKindDiff,
				Digest: "sha256:feed",
			},
			LogArtifact: step.PublishedArtifact{
				CID:    "bafy-logs-apply",
				Kind:   step.ArtifactKindLogs,
				Digest: "sha256:babe",
			},
			GateArtifact: step.PublishedArtifact{
				CID:    "bafy-gate-apply",
				Kind:   step.ArtifactKindGateReport,
				Digest: "sha256:abba",
			},
			GateReport: step.GateResult{
				Passed: true,
			},
			Retained:     true,
			RetentionTTL: "36h",
		},
	}
	client, err := NewLocalStepClient(LocalStepClientOptions{Runner: stepRunner})
	if err != nil {
		t.Fatalf("NewLocalStepClient() error: %v", err)
	}
	stage := runner.Stage{
		Name:         "mods-apply",
		Kind:         runner.StageKindModsORWApply,
		Lane:         "mods-orw",
		StepManifest: &manifest,
	}
	ticket := contracts.WorkflowTicket{TicketID: "ticket-456"}

	outcome, err := client.ExecuteStage(context.Background(), ticket, stage, "/tmp/workspace")
	if err != nil {
		t.Fatalf("ExecuteStage() unexpected error: %v", err)
	}
	if outcome.Status != runner.StageStatusCompleted {
		t.Fatalf("expected completed outcome, got %s", outcome.Status)
	}

	invocations := client.Invocations()
	if len(invocations) != 1 {
		t.Fatalf("expected single invocation, got %d", len(invocations))
	}
	inv := invocations[0]
	if inv.Stage.Name != stage.Name {
		t.Fatalf("expected invocation stage %q, got %q", stage.Name, inv.Stage.Name)
	}
	if inv.RunID != stepRunner.result.ContainerID {
		t.Fatalf("expected run id %q, got %q", stepRunner.result.ContainerID, inv.RunID)
	}
	if len(inv.Artifacts) != 3 {
		t.Fatalf("expected artifacts recorded, got %d", len(inv.Artifacts))
	}
	if !containsArtifact(inv.Artifacts, "diff", "bafy-diff-apply") {
		t.Fatalf("expected diff artifact recorded, got %+v", inv.Artifacts)
	}
	if !containsArtifact(inv.Artifacts, "logs", "bafy-logs-apply") {
		t.Fatalf("expected log artifact recorded, got %+v", inv.Artifacts)
	}
	if !containsArtifact(inv.Artifacts, "gate_report", "bafy-gate-apply") {
		t.Fatalf("expected gate artifact recorded, got %+v", inv.Artifacts)
	}
	if inv.Evidence == nil {
		t.Fatalf("expected evidence recorded")
	}
	if inv.Evidence.Metadata["retained"] != "true" {
		t.Fatalf("expected retention metadata, got %+v", inv.Evidence.Metadata)
	}
	if inv.Evidence.Metadata["retention_ttl"] != "36h" {
		t.Fatalf("expected retention ttl metadata, got %+v", inv.Evidence.Metadata)
	}
}
