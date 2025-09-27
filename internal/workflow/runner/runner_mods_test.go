package runner_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestRunPublishesModsMetadata(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	manifest := defaultManifestCompilation()
	plan := runner.ExecutionPlan{
		TicketID: "ticket-123",
		Stages: []runner.Stage{
			{
				Name:        mods.StageNamePlan,
				Kind:        runner.StageKindModsPlan,
				Lane:        "node-wasm",
				Constraints: runner.StageConstraints{Manifest: manifest},
				Metadata: runner.StageMetadata{Mods: &runner.StageModsMetadata{
					Plan: &runner.StageModsPlan{
						SelectedRecipes: []string{"recipe.alpha"},
						ParallelStages:  []string{mods.StageNameORWApply, mods.StageNameORWGenerate},
						HumanGate:       true,
						Summary:         "knowledge base recommends prompting human review",
					},
					Recommendations: []runner.StageModsRecommendation{{
						Source:     "knowledge-base",
						Message:    "Apply recipe.alpha before llm-exec",
						Confidence: 1.5,
					}, {
						Source:     "knowledge-base",
						Message:    " ",
						Confidence: 0.25,
					}},
				}},
			},
			{
				Name:         mods.StageNameHuman,
				Kind:         runner.StageKindModsHuman,
				Lane:         "go-native",
				Dependencies: []string{mods.StageNamePlan},
				Constraints:  runner.StageConstraints{Manifest: manifest},
				Metadata: runner.StageMetadata{Mods: &runner.StageModsMetadata{
					Human: &runner.StageModsHuman{
						Required:  true,
						Playbooks: []string{"playbook.mods.review"},
					},
				}},
			},
		},
	}
	planner := metadataPlanner{plan: plan}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			mods.StageNamePlan: {
				{Status: runner.StageStatusRunning},
				{Status: runner.StageStatusCompleted},
			},
			mods.StageNameHuman: {
				{Status: runner.StageStatusCompleted},
			},
		},
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          planner,
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var planRunning, planCompleted, humanCompleted *contracts.WorkflowCheckpoint
	for i := range events.checkpoints {
		cp := events.checkpoints[i]
		if cp.Stage != mods.StageNamePlan && cp.Stage != mods.StageNameHuman {
			continue
		}
		switch cp.Stage {
		case mods.StageNamePlan:
			switch cp.Status {
			case contracts.CheckpointStatusRunning:
				planRunning = &cp
			case contracts.CheckpointStatusCompleted:
				planCompleted = &cp
			}
		case mods.StageNameHuman:
			if cp.Status == contracts.CheckpointStatusCompleted {
				humanCompleted = &cp
			}
		}
	}
	if planRunning == nil || planCompleted == nil || humanCompleted == nil {
		t.Fatalf("expected mods checkpoints recorded, got running=%#v completed=%#v human=%#v", planRunning, planCompleted, humanCompleted)
	}
	if planRunning.StageMetadata == nil || planRunning.StageMetadata.Mods == nil {
		t.Fatalf("expected mods metadata on running checkpoint: %#v", planRunning.StageMetadata)
	}
	if planCompleted.StageMetadata == nil || planCompleted.StageMetadata.Mods == nil {
		t.Fatalf("expected mods metadata on completed checkpoint: %#v", planCompleted.StageMetadata)
	}
	planMeta := planCompleted.StageMetadata.Mods
	if planMeta.Plan == nil {
		t.Fatalf("expected plan metadata present: %#v", planMeta)
	}
	if len(planMeta.Plan.SelectedRecipes) != 1 || planMeta.Plan.SelectedRecipes[0] != "recipe.alpha" {
		t.Fatalf("unexpected plan recipes: %#v", planMeta.Plan.SelectedRecipes)
	}
	if len(planMeta.Plan.ParallelStages) != 2 {
		t.Fatalf("unexpected plan parallel stages: %#v", planMeta.Plan.ParallelStages)
	}
	if !planMeta.Plan.HumanGate {
		t.Fatalf("expected plan metadata to flag human gate")
	}
	if planMeta.Plan.Summary == "" {
		t.Fatalf("expected plan summary present")
	}
	if len(planMeta.Recommendations) != 1 || planMeta.Recommendations[0].Message == "" {
		t.Fatalf("expected plan recommendations recorded: %#v", planMeta.Recommendations)
	}
	if planMeta.Recommendations[0].Confidence != 1 {
		t.Fatalf("expected recommendation confidence clamped to 1, got %#v", planMeta.Recommendations[0].Confidence)
	}

	if humanCompleted.StageMetadata.Mods == nil || humanCompleted.StageMetadata.Mods.Human == nil {
		t.Fatalf("expected human metadata recorded: %#v", humanCompleted.StageMetadata)
	}
	humanMeta := humanCompleted.StageMetadata.Mods.Human
	if !humanMeta.Required {
		t.Fatalf("expected human stage required")
	}
	if len(humanMeta.Playbooks) != 1 || humanMeta.Playbooks[0] != "playbook.mods.review" {
		t.Fatalf("expected human playbook recorded: %#v", humanMeta.Playbooks)
	}
}

func TestRunPublishesModsPlannerHints(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-hints", tenant: "acme"}
	grid := &fakeGrid{}
	opts := runner.Options{
		Ticket:           "ticket-hints",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: newStubCompiler(),
	}
	setRunnerModsOptions(t, &opts, 75*time.Second, 4)

	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var planCompleted *contracts.WorkflowCheckpoint
	for i := range events.checkpoints {
		cp := events.checkpoints[i]
		if cp.Stage == mods.StageNamePlan && cp.Status == contracts.CheckpointStatusCompleted {
			planCompleted = &cp
			break
		}
	}
	if planCompleted == nil {
		t.Fatalf("expected completed plan checkpoint, got %#v", events.checkpoints)
	}
	if planCompleted.StageMetadata == nil || planCompleted.StageMetadata.Mods == nil {
		t.Fatalf("expected mods metadata on plan checkpoint: %#v", planCompleted.StageMetadata)
	}
	if planCompleted.StageMetadata.Mods.Plan == nil {
		t.Fatalf("expected mods plan metadata present: %#v", planCompleted.StageMetadata.Mods)
	}
	planValue := reflect.ValueOf(*planCompleted.StageMetadata.Mods.Plan)
	planTimeoutField := planValue.FieldByName("PlanTimeout")
	if !planTimeoutField.IsValid() {
		t.Fatalf("plan metadata missing PlanTimeout field: %#v", planCompleted.StageMetadata.Mods.Plan)
	}
	if planTimeoutField.String() != "1m15s" {
		t.Fatalf("expected plan timeout 1m15s, got %q", planTimeoutField.String())
	}
	maxParallelField := planValue.FieldByName("MaxParallel")
	if !maxParallelField.IsValid() {
		t.Fatalf("plan metadata missing MaxParallel field: %#v", planCompleted.StageMetadata.Mods.Plan)
	}
	if int(maxParallelField.Int()) != 4 {
		t.Fatalf("expected max parallel 4, got %d", maxParallelField.Int())
	}

	planInvocation := findStageCall(grid.calls, mods.StageNamePlan)
	if planInvocation.stage.Metadata.Mods == nil || planInvocation.stage.Metadata.Mods.Plan == nil {
		t.Fatalf("expected mods plan metadata on grid invocation: %#v", planInvocation.stage.Metadata)
	}
	invokedPlan := planInvocation.stage.Metadata.Mods.Plan
	if invokedPlan.PlanTimeout != "1m15s" {
		t.Fatalf("expected grid invocation plan timeout 1m15s, got %q", invokedPlan.PlanTimeout)
	}
	if invokedPlan.MaxParallel != 4 {
		t.Fatalf("expected grid invocation max parallel 4, got %d", invokedPlan.MaxParallel)
	}
}
