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
events := &recordingEvents{nextTicket: "ticket-123"}
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
    // tenant removed
		Events:           events,
		Grid:             grid,
		Planner:          planner,
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		calls := grid.callsSnapshot()
		names := make([]string, len(calls))
		for i := range calls {
			names[i] = calls[i].stage.Name
		}
		t.Fatalf("unexpected error: %v (stages=%v)", err, names)
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
events := &recordingEvents{nextTicket: "ticket-hints"}
	grid := &fakeGrid{}
	opts := runner.Options{
		Ticket:           "ticket-hints",
    // tenant removed
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

func TestRunExecutesParallelModsStages(t *testing.T) {
events := &recordingEvents{nextTicket: "ticket-parallel"}
	grid := newParallelRecordingGrid()
	grid.addGate(mods.StageNameORWApply)
	grid.addGate(mods.StageNameORWGenerate)
	defer func() {
		grid.allow(mods.StageNameORWApply)
		grid.allow(mods.StageNameORWGenerate)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	opts := runner.Options{
		Ticket:           "ticket-parallel",
    // tenant removed
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: newStubCompiler(),
	}

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, opts)
	}()

	const stageStartTimeout = 5 * time.Second
	if !grid.waitForStart(mods.StageNameORWApply, stageStartTimeout) {
		t.Fatalf("expected orw-apply to start")
	}
	if !grid.waitForStart(mods.StageNameORWGenerate, stageStartTimeout) {
		t.Fatalf("expected orw-gen to start while orw-apply running")
	}

	grid.allow(mods.StageNameORWApply)
	grid.allow(mods.StageNameORWGenerate)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatalf("runner did not finish")
	}
}

func TestRunRetriesParallelModsStageBeforeDependents(t *testing.T) {
events := &recordingEvents{nextTicket: "ticket-parallel-retry"}
	grid := newParallelRecordingGrid()
	grid.setOutcomes(mods.StageNameORWApply, []runner.StageOutcome{
		{Status: runner.StageStatusFailed, Retryable: true, Message: "planner detected failure"},
		{Status: runner.StageStatusCompleted},
	})
	grid.setOutcomes(mods.StageNameORWGenerate, []runner.StageOutcome{{Status: runner.StageStatusCompleted}})

	opts := runner.Options{
		Ticket:           "ticket-parallel-retry",
    // tenant removed
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}

	if err := runner.Run(context.Background(), opts); err != nil {
		calls := grid.callsSnapshot()
		names := make([]string, len(calls))
		for i := range calls {
			names[i] = calls[i].stage.Name
		}
		t.Fatalf("unexpected error: %v (stages=%v)", err, names)
	}
	calls := grid.callsSnapshot()
	if attempts := gatherStageAttempts(calls, mods.StageNameORWApply); attempts != 2 {
		t.Fatalf("expected two attempts for orw-apply, got %d", attempts)
	}
	llmExecIndex := findStageIndex(calls, mods.StageNameLLMExec)
	if llmExecIndex == -1 {
		t.Fatalf("expected llm-exec call recorded")
	}
	lastApplyIndex := findLastStageIndex(calls, mods.StageNameORWApply)
	if lastApplyIndex == -1 {
		t.Fatalf("expected orw-apply call recorded")
	}
	if llmExecIndex < lastApplyIndex {
		t.Fatalf("llm-exec invoked before final orw-apply completion: calls=%#v", calls)
	}

	sawRetry := false
	for _, cp := range events.checkpoints {
		if cp.Stage == mods.StageNameORWApply && cp.Status == contracts.CheckpointStatusRetrying {
			sawRetry = true
			break
		}
	}
	if !sawRetry {
		t.Fatalf("expected retry checkpoint for orw-apply")
	}
}

func TestRunSchedulesHealingPlanAfterBuildGateFailure(t *testing.T) {
events := &recordingEvents{nextTicket: "ticket-buildgate-heal"}
	grid := &fakeGrid{}
	grid.setOutcomes(buildGateStage, []runner.StageOutcome{{Status: runner.StageStatusFailed, Retryable: true, Message: "compile failed"}})
	grid.setOutcomes(buildGateStage+"#heal1", []runner.StageOutcome{{Status: runner.StageStatusCompleted}})
	grid.setOutcomes(staticChecksStage+"#heal1", []runner.StageOutcome{{Status: runner.StageStatusCompleted}})
	grid.setOutcomes("test#heal1", []runner.StageOutcome{{Status: runner.StageStatusCompleted}})

	modsOpts := runner.ModsOptions{
		PlanTimeout:     30 * time.Second,
		MaxParallel:     2,
		PlanLane:        "mods-plan",
		OpenRewriteLane: "mods-java",
		LLMPlanLane:     "mods-llm",
		LLMExecLane:     "mods-llm",
		HumanLane:       "mods-human",
	}
	opts := runner.Options{
		Ticket:           "",
    // tenant removed
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlannerWithMods(modsOpts),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: newStubCompiler(),
		Mods:             modsOpts,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		calls := grid.callsSnapshot()
		names := make([]string, len(calls))
		for i := range calls {
			names[i] = calls[i].stage.Name
		}
		t.Fatalf("unexpected error: %v (stages=%v)", err, names)
	}

	calls := grid.callsSnapshot()
	if findStageIndex(calls, buildGateStage) == -1 {
		t.Fatalf("expected initial build-gate invocation recorded: %#v", calls)
	}
	if findStageIndex(calls, buildGateStage+"#heal1") == -1 {
		t.Fatalf("expected healing build-gate invocation recorded: %#v", calls)
	}
	if findStageIndex(calls, mods.StageNamePlan+"#heal1") == -1 {
		t.Fatalf("expected healing mods-plan recorded: %#v", calls)
	}
}

func findStageIndex(calls []gridCall, stage string) int {
	for i, call := range calls {
		if call.stage.Name == stage {
			return i
		}
	}
	return -1
}

func findLastStageIndex(calls []gridCall, stage string) int {
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].stage.Name == stage {
			return i
		}
	}
	return -1
}
