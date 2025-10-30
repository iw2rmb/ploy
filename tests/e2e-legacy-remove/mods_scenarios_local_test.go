//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestModsScenarioSimpleOpenRewrite(t *testing.T) {
	scenario := mustScenario(t, "simple-openrewrite")
	harness := newScenarioHarness(t, scenario, scenarioOptions{
		Advice: mods.Advice{
			Plan: mods.AdvicePlan{
				SelectedRecipes: []string{"org.openrewrite.java.UpgradeJavaVersion"},
				Summary:         "Upgrade project to JDK17",
			},
			Recommendations: []mods.AdviceRecommendation{{
				Source:     "knowledge-base",
				Message:    "Review Gradle wrapper after OpenRewrite run",
				Confidence: 0.72,
			}},
		},
	})

	if err := harness.run(); err != nil {
		for _, invocation := range harness.recorder.Invocations() {
			runtime := strings.TrimSpace(invocation.Stage.Job.Runtime)
			if runtime == "" {
				runtime = "(unset)"
			}
			if invocation.Evidence != nil {
				exit := "(nil)"
				if invocation.Evidence.ExitCode != nil {
					exit = fmt.Sprintf("%d", *invocation.Evidence.ExitCode)
				}
				t.Logf("stage=%s lane=%s runtime=%s run=%s state=%s exit=%s source=%s",
					invocation.Stage.Name, invocation.Stage.Lane, runtime, invocation.RunID,
					invocation.Evidence.JobState, exit, invocation.Evidence.Source)
			} else {
				t.Logf("stage=%s lane=%s runtime=%s run=%s (no evidence)", invocation.Stage.Name, invocation.Stage.Lane, runtime, invocation.RunID)
			}
		}
		t.Fatalf("mods scenario simple-openrewrite failed: %v", err)
	}

	names := harness.stageNames()
	expected := []string{
		mods.StageNamePlan,
		mods.StageNameORWApply,
		mods.StageNameORWGenerate,
		mods.StageNameLLMPlan,
		mods.StageNameLLMExec,
		mods.StageNameHuman,
		"build-gate",
		"static-checks",
		"test",
	}
	assertStageSet(t, names, expected)
	if containsHealing(names) {
		t.Fatalf("expected simple scenario to avoid healing, got stages: %v", names)
	}

	if idx(mods.StageNameHuman, names) > idx("build-gate", names) {
		t.Fatalf("expected build-gate to execute after mods-human, order=%v", names)
	}

	requests := harness.workspaceRequests()
	if len(requests) != 1 {
		t.Fatalf("expected workspace preparer invoked once, got %d", len(requests))
	}
	repo := requests[0].Ticket.Repo
	if repo.URL == "" || repo.TargetRef == "" {
		t.Fatalf("workspace prep missing repo metadata: %#v", repo)
	}

	calls := harness.advisorRequests()
	if len(calls) != 1 {
		t.Fatalf("expected single advisor invocation, got %d", len(calls))
	}
	if len(calls[0].Signals.Errors) != 0 {
		t.Fatalf("expected no healing signals, got %#v", calls[0].Signals.Errors)
	}

	var planInvocation *runner.StageInvocation
	for _, invocation := range harness.recorder.Invocations() {
		if invocation.Stage.Name == mods.StageNamePlan {
			plan := invocation
			planInvocation = &plan
			break
		}
	}
	if planInvocation == nil {
		t.Fatalf("mods-plan invocation missing from recorder")
	}
	if planInvocation.Evidence == nil {
		t.Fatalf("expected mods-plan evidence to be collected")
	}
	evidence := planInvocation.Evidence
	if strings.TrimSpace(evidence.JobState) == "" {
		t.Fatalf("expected job state to be recorded, got %#v", evidence.JobState)
	}
	if evidence.ExitCode == nil || *evidence.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %#v", evidence.ExitCode)
	}
	if strings.TrimSpace(evidence.LogTail) == "" {
		t.Fatalf("expected log tail to be captured, got %q", evidence.LogTail)
	}
	if len(evidence.Events) == 0 {
		t.Fatalf("expected workflow events to be captured, got %#v", evidence.Events)
	}
}

func TestModsScenarioBuildGateSelfHeal(t *testing.T) {
	scenario := mustScenario(t, "buildgate-self-heal")
	harness := newScenarioHarness(t, scenario, scenarioOptions{
		Advice: mods.Advice{
			Plan: mods.AdvicePlan{
				SelectedRecipes: []string{"org.openrewrite.java.AddMissingDependencies"},
				Summary:         "Retry build after dependency fix",
				HumanGate:       true,
				ParallelStages:  []string{mods.StageNameORWApply, mods.StageNameLLMExec},
			},
			Human: mods.AdviceHuman{
				Required:  true,
				Playbooks: []string{"playbooks/mods/manual-check.md"},
			},
			Recommendations: []mods.AdviceRecommendation{{
				Source:     "knowledge-base",
				Message:    "Apply missing symbol recipe",
				Confidence: 0.84,
			}},
		},
	})

	if err := harness.run(); err != nil {
		t.Fatalf("mods scenario buildgate-self-heal failed: %v", err)
	}

	names := harness.stageNames()
	if !containsHealing(names) {
		t.Fatalf("expected healing branch to execute, stages=%v", names)
	}

	for _, required := range []string{
		mods.StageNamePlan + "#heal1",
		mods.StageNameORWApply + "#heal1",
		mods.StageNameLLMExec + "#heal1",
		mods.StageNameHuman + "#heal1",
		"build-gate#heal1",
		"static-checks#heal1",
		"test#heal1",
	} {
		if idx(required, names) == -1 {
			t.Fatalf("expected healing stage %s not invoked (stages=%v)", required, names)
		}
	}

	calls := harness.advisorRequests()
	if len(calls) != 2 {
		t.Fatalf("expected advisor invoked twice, got %d", len(calls))
	}
	if len(calls[1].Signals.Errors) == 0 {
		t.Fatalf("expected healing signals to include build gate failure, got %#v", calls[1].Signals.Errors)
	}

	stage, ok := harness.stageByName(mods.StageNamePlan + "#heal1")
	if !ok {
		t.Fatalf("healing mods-plan stage missing")
	}
	if stage.Metadata.Mods == nil || stage.Metadata.Mods.Plan == nil {
		t.Fatalf("expected mods metadata on healing plan: %#v", stage.Metadata)
	}
	planMeta := stage.Metadata.Mods.Plan
	if !planMeta.HumanGate {
		t.Fatalf("expected healing plan to require human gate")
	}
	if !containsValue(planMeta.ParallelStages, mods.StageNameLLMExec) {
		t.Fatalf("expected parallel stage metadata to include llm-exec, got %#v", planMeta.ParallelStages)
	}
}

func TestModsScenarioParallelHealingOptions(t *testing.T) {
	scenario := mustScenario(t, "parallel-healing-options")
	harness := newScenarioHarness(t, scenario, scenarioOptions{
		Advice: mods.Advice{
			Plan: mods.AdvicePlan{
				SelectedRecipes: []string{"org.openrewrite.java.ReplaceDeprecatedApi"},
				Summary:         "Coordinate parallel fixes",
				HumanGate:       true,
				ParallelStages:  []string{mods.StageNameORWApply, mods.StageNameLLMExec},
			},
			Human: mods.AdviceHuman{
				Required:  true,
				Playbooks: []string{"playbooks/mods/parallel-review.md"},
			},
		},
		PlanTimeout:     2*time.Minute + 30*time.Second,
		ModsMaxParallel: 3,
	})

	if err := harness.run(); err != nil {
		t.Fatalf("mods scenario parallel-healing-options failed: %v", err)
	}

	names := harness.stageNames()
	if !containsHealing(names) {
		t.Fatalf("expected healing branch to execute, stages=%v", names)
	}

	planStage, ok := harness.stageByName(mods.StageNamePlan + "#heal1")
	if !ok {
		t.Fatalf("healing plan stage missing")
	}

	planMeta := planStage.Metadata.Mods.Plan
	if planMeta == nil {
		t.Fatalf("expected plan metadata on healing stage")
	}
	if planMeta.MaxParallel != 3 {
		t.Fatalf("expected plan max parallel 3, got %d", planMeta.MaxParallel)
	}
	if strings.TrimSpace(planMeta.PlanTimeout) != "2m30s" {
		t.Fatalf("expected plan timeout 2m30s, got %q", planMeta.PlanTimeout)
	}
	if !containsValue(planMeta.ParallelStages, mods.StageNameLLMExec) {
		t.Fatalf("expected parallel stages to include llm-exec, got %#v", planMeta.ParallelStages)
	}
}
