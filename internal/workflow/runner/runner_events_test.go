package runner_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestRunReturnsClaimTicketError(t *testing.T) {
	events := &errorEvents{claimErr: errors.New("claim failed")}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             runner.NewInMemoryGrid(),
		Planner:          runner.NewDefaultPlanner(),
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.claimErr) {
		t.Fatalf("expected claim error, got %v", err)
	}
}

func TestRunPropagatesPublishCheckpointError(t *testing.T) {
	events := &errorEvents{
		ticket: contracts.WorkflowTicket{
			SchemaVersion: contracts.SchemaVersion,
			TicketID:      "ticket-123",
			Tenant:        "acme",
		},
		publishErr: errors.New("checkpoint failure"),
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             runner.NewInMemoryGrid(),
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.publishErr) {
		t.Fatalf("expected publish error, got %v", err)
	}
}

func TestRunPropagatesPublishArtifactError(t *testing.T) {
	events := &errorEvents{
		ticket: contracts.WorkflowTicket{
			SchemaVersion: contracts.SchemaVersion,
			TicketID:      "ticket-123",
			Tenant:        "acme",
		},
		artifactErr: errors.New("artifact failure"),
	}
	grid := runner.NewInMemoryGrid()
	grid.StageOutcomes[modsPlanStage] = []runner.StageOutcome{{
		Status: runner.StageStatusCompleted,
		Artifacts: []runner.Artifact{{
			Name:        "mods-plan",
			ArtifactCID: "cid-mods-plan",
		}},
	}}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.artifactErr) {
		t.Fatalf("expected artifact publish error, got %v", err)
	}
}

func TestRunPublishesCacheKeysInCheckpoints(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := runner.NewInMemoryGrid()
	composer := &recordingCacheComposer{}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
		CacheComposer:    composer,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(composer.calls) == 0 {
		t.Fatal("expected cache composer to be invoked")
	}
	stageChecks := map[string]int{modsPlanStage: 0, buildGateStage: 0, staticChecksStage: 0, "test": 0}
	for _, checkpoint := range events.checkpoints {
		switch checkpoint.Stage {
		case modsPlanStage, buildGateStage, staticChecksStage, "test":
			if checkpoint.CacheKey == "" {
				t.Fatalf("expected cache key for stage %s", checkpoint.Stage)
			}
			expected := fmt.Sprintf("cache-%s", checkpoint.Stage)
			if checkpoint.CacheKey != expected {
				t.Fatalf("unexpected cache key for %s: %s", checkpoint.Stage, checkpoint.CacheKey)
			}
			stageChecks[checkpoint.Stage]++
		case "ticket-claimed", "workflow":
			if checkpoint.CacheKey != "" {
				t.Fatalf("expected no cache key for %s checkpoint", checkpoint.Stage)
			}
		}
	}
	for stage, count := range stageChecks {
		if count == 0 {
			t.Fatalf("expected cache key checkpoints for stage %s", stage)
		}
	}
}

func TestRunPublishesStageMetadataAndArtifacts(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			ManifestVersion: "v2",
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			},
		},
	}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{
				Status:    runner.StageStatusCompleted,
				Artifacts: []runner.Artifact{{Name: "mods-plan", ArtifactCID: "cid-mods-plan", Digest: "sha256:modsplan", MediaType: "application/tar+zst"}},
			}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: compiler,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var modsRunning, modsCompleted *contracts.WorkflowCheckpoint
	for i := range events.checkpoints {
		cp := events.checkpoints[i]
		if cp.Stage != modsPlanStage {
			continue
		}
		switch cp.Status {
		case contracts.CheckpointStatusRunning:
			modsRunning = &cp
		case contracts.CheckpointStatusCompleted:
			modsCompleted = &cp
		}
	}
	if modsRunning == nil || modsRunning.StageMetadata == nil {
		t.Fatalf("expected running checkpoint with stage metadata: %#v", modsRunning)
	}
	if modsRunning.StageMetadata.Lane != "node-wasm" {
		t.Fatalf("unexpected lane on running checkpoint: %#v", modsRunning.StageMetadata)
	}
	if len(modsRunning.Artifacts) > 0 {
		t.Fatalf("expected no artifacts on running checkpoint: %#v", modsRunning.Artifacts)
	}
	if modsCompleted == nil {
		t.Fatal("expected completed mods checkpoint")
	}
	if modsCompleted.StageMetadata == nil {
		t.Fatalf("expected stage metadata on completed checkpoint: %#v", modsCompleted)
	}
	if modsCompleted.StageMetadata.Manifest.Version != "2025-09-26" {
		t.Fatalf("unexpected manifest on completed checkpoint: %#v", modsCompleted.StageMetadata.Manifest)
	}
	if len(modsCompleted.Artifacts) != 1 {
		t.Fatalf("expected single artifact on completed checkpoint: %#v", modsCompleted.Artifacts)
	}
	artifact := modsCompleted.Artifacts[0]
	if artifact.ArtifactCID != "cid-mods-plan" || artifact.Digest != "sha256:modsplan" {
		t.Fatalf("unexpected artifact manifest: %#v", artifact)
	}

	if len(events.artifacts) != 1 {
		t.Fatalf("expected single artifact envelope, got %d", len(events.artifacts))
	}
	envelope := events.artifacts[0]
	if envelope.TicketID != "ticket-123" {
		t.Fatalf("unexpected artifact ticket id: %#v", envelope)
	}
	if envelope.Stage != modsPlanStage {
		t.Fatalf("unexpected artifact stage: %#v", envelope)
	}
	if envelope.Artifact.ArtifactCID != "cid-mods-plan" {
		t.Fatalf("expected artifact CID to mirror checkpoint, got %#v", envelope.Artifact)
	}
	if envelope.StageMetadata == nil || envelope.StageMetadata.Lane != "node-wasm" {
		t.Fatalf("expected artifact envelope to include stage metadata: %#v", envelope.StageMetadata)
	}

	var workflowCheckpoint *contracts.WorkflowCheckpoint
	for i := range events.checkpoints {
		cp := events.checkpoints[i]
		if cp.Stage == "workflow" && cp.Status == contracts.CheckpointStatusCompleted {
			workflowCheckpoint = &cp
			break
		}
	}
	if workflowCheckpoint == nil {
		t.Fatal("expected workflow completion checkpoint")
	}
	if workflowCheckpoint.StageMetadata != nil {
		t.Fatalf("expected workflow checkpoint to omit stage metadata: %#v", workflowCheckpoint.StageMetadata)
	}
	if len(workflowCheckpoint.Artifacts) != 0 {
		t.Fatalf("expected workflow checkpoint to omit artifacts: %#v", workflowCheckpoint.Artifacts)
	}
}

func TestRunPublishesBuildGateMetadata(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := newStubCompiler()
	buildGateMetadata := runner.StageMetadata{
		BuildGate: &runner.StageBuildGateMetadata{
			LogDigest: "  bafy-build-log  ",
			StaticChecks: []runner.StageStaticCheck{
				{
					Language: " Go ",
					Tool:     " go vet ",
					Passed:   false,
					Failures: []runner.StageStaticCheckFailure{
						{
							RuleID:   " GOVET001 ",
							File:     " ./main.go ",
							Line:     -5,
							Column:   3,
							Severity: "ERROR",
							Message:  " unused import ",
						},
					},
				},
			},
			LogFindings: []runner.StageLogFinding{
				{
					Code:     " kb.git.auth ",
					Severity: "ERROR",
					Message:  " Authenticate Git fetch credentials for remote repository access. ",
					Evidence: " fatal: unable to access 'https://example.com/repo' ",
				},
			},
		},
	}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: []runner.StageOutcome{{Status: runner.StageStatusCompleted}},
			buildGateStage: []runner.StageOutcome{
				{
					Status: runner.StageStatusCompleted,
					Stage: runner.Stage{
						Name:     buildGateStage,
						Kind:     runner.StageKindBuildGate,
						Lane:     "go-native",
						Metadata: buildGateMetadata,
					},
				},
			},
			staticChecksStage: []runner.StageOutcome{{Status: runner.StageStatusCompleted}},
			"test":            []runner.StageOutcome{{Status: runner.StageStatusCompleted}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buildGateCheckpoint *contracts.WorkflowCheckpoint
	for i := range events.checkpoints {
		cp := events.checkpoints[i]
		if cp.Stage != buildGateStage || cp.Status != contracts.CheckpointStatusCompleted {
			continue
		}
		buildGateCheckpoint = &cp
		break
	}
	if buildGateCheckpoint == nil {
		t.Fatal("expected build gate checkpoint to be published")
	}
	if buildGateCheckpoint.StageMetadata == nil || buildGateCheckpoint.StageMetadata.BuildGate == nil {
		t.Fatalf("expected build gate metadata on checkpoint: %#v", buildGateCheckpoint.StageMetadata)
	}
	meta := buildGateCheckpoint.StageMetadata.BuildGate
	if meta.LogDigest != "bafy-build-log" {
		t.Fatalf("expected trimmed digest, got %q", meta.LogDigest)
	}
	if len(meta.StaticChecks) != 1 {
		t.Fatalf("expected single static check metadata, got %d", len(meta.StaticChecks))
	}
	report := meta.StaticChecks[0]
	if report.Language != "Go" {
		t.Fatalf("expected trimmed language, got %q", report.Language)
	}
	if report.Tool != "go vet" {
		t.Fatalf("expected trimmed tool, got %q", report.Tool)
	}
	if report.Passed {
		t.Fatal("expected static check report to remain failed")
	}
	if len(report.Failures) != 1 {
		t.Fatalf("expected single failure, got %d", len(report.Failures))
	}
	failure := report.Failures[0]
	if failure.RuleID != "GOVET001" {
		t.Fatalf("expected trimmed rule id, got %q", failure.RuleID)
	}
	if failure.File != "./main.go" {
		t.Fatalf("expected trimmed file, got %q", failure.File)
	}
	if failure.Line != 0 {
		t.Fatalf("expected clamped line 0, got %d", failure.Line)
	}
	if failure.Column != 3 {
		t.Fatalf("expected column to remain 3, got %d", failure.Column)
	}
	if failure.Severity != "error" {
		t.Fatalf("expected severity lower-cased error, got %q", failure.Severity)
	}
	if failure.Message != "unused import" {
		t.Fatalf("expected trimmed message, got %q", failure.Message)
	}
	if len(meta.LogFindings) != 1 {
		t.Fatalf("expected single log finding, got %d", len(meta.LogFindings))
	}
	finding := meta.LogFindings[0]
	if finding.Code != "kb.git.auth" {
		t.Fatalf("expected trimmed code, got %q", finding.Code)
	}
	if finding.Severity != "error" {
		t.Fatalf("expected severity lower-cased error, got %q", finding.Severity)
	}
	if finding.Message != "Authenticate Git fetch credentials for remote repository access." {
		t.Fatalf("unexpected message: %q", finding.Message)
	}
	if finding.Evidence != "fatal: unable to access 'https://example.com/repo'" {
		t.Fatalf("unexpected evidence: %q", finding.Evidence)
	}
}

func TestRunFailsWhenStageCompletionPublishFails(t *testing.T) {
	events := &countingEvents{
		ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme"},
		failAt: 3,
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             noStageGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.err) {
		t.Fatalf("expected publish error, got %v", err)
	}
}

func TestRunFailsWhenFinalPublishFails(t *testing.T) {
	events := &countingEvents{
		ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme"},
		failAt: 8,
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             noStageGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.err) {
		t.Fatalf("expected final publish error, got %v", err)
	}
}

func TestRunAutoClaimsTicketAndCleansWorkspace(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage:  {{Status: runner.StageStatusCompleted}},
			buildGateStage: {{Status: runner.StageStatusCompleted}},
			"test":         {{Status: runner.StageStatusCompleted}},
		},
	}
	planner := runner.NewDefaultPlanner()
	workspaceRoot := t.TempDir()
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          planner,
		WorkspaceRoot:    workspaceRoot,
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if len(events.claimedTickets) != 1 || events.claimedTickets[0] != "ticket-123" {
		t.Fatalf("expected auto-claimed ticket, got %v", events.claimedTickets)
	}
	sequence := extractStageStatuses(events.checkpoints)
	if len(sequence) == 0 || sequence[0].stage != "ticket-claimed" || sequence[0].status != runner.StageStatusCompleted {
		t.Fatalf("expected ticket-claimed checkpoint first, got %v", sequence)
	}
	requireStageStatuses(t, sequence, modsPlanStage, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameORWApply, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameORWGenerate, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameLLMPlan, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameLLMExec, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameHuman, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, buildGateStage, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, staticChecksStage, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, "test", []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	workflowStatuses := collectStageStatuses(sequence, "workflow")
	if len(workflowStatuses) != 1 || workflowStatuses[0] != runner.StageStatusCompleted {
		t.Fatalf("expected workflow completion, got %v", workflowStatuses)
	}
	if grid.lastWorkspace == "" {
		t.Fatal("expected workspace to be recorded")
	}
	if !strings.HasPrefix(grid.lastWorkspace, workspaceRoot) {
		t.Fatalf("workspace %q not under root %q", grid.lastWorkspace, workspaceRoot)
	}
	if _, err := os.Stat(grid.lastWorkspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected workspace to be deleted, stat err=%v", err)
	}
}
