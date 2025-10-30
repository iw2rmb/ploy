package mods_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/knowledgebase"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

func TestPlannerBuildsModsStageGraph(t *testing.T) {
	planner := mods.NewPlanner(mods.Options{
		PlanLane:        "node-wasm",
		OpenRewriteLane: "node-wasm",
		LLMPlanLane:     "gpu-ml",
		LLMExecLane:     "gpu-ml",
		HumanLane:       "mods-human",
	})
	stages, err := planner.Plan(context.Background(), mods.PlanInput{
        Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123"},
	})
	if err != nil {
		t.Fatalf("unexpected error planning mods stages: %v", err)
	}
	if len(stages) != 6 {
		t.Fatalf("expected 6 stages, got %d", len(stages))
	}
	expect := []struct {
		name string
		kind string
		lane string
		deps []string
	}{
		{mods.StageNamePlan, mods.StageNamePlan, "node-wasm", nil},
		{mods.StageNameORWApply, mods.StageNameORWApply, "node-wasm", []string{mods.StageNamePlan}},
		{mods.StageNameORWGenerate, mods.StageNameORWGenerate, "node-wasm", []string{mods.StageNamePlan}},
		{mods.StageNameLLMPlan, mods.StageNameLLMPlan, "gpu-ml", []string{mods.StageNamePlan}},
		{mods.StageNameLLMExec, mods.StageNameLLMExec, "gpu-ml", []string{mods.StageNameORWApply, mods.StageNameORWGenerate, mods.StageNameLLMPlan}},
		{mods.StageNameHuman, mods.StageNameHuman, "mods-human", []string{mods.StageNameLLMExec}},
	}
	for i, exp := range expect {
		stage := stages[i]
		if stage.Name != exp.name {
			t.Fatalf("unexpected stage %d name: got %s want %s", i, stage.Name, exp.name)
		}
		if stage.Kind != exp.kind {
			t.Fatalf("unexpected stage %s kind: got %s want %s", stage.Name, stage.Kind, exp.kind)
		}
		if stage.Lane != exp.lane {
			t.Fatalf("unexpected stage %s lane: got %s want %s", stage.Name, stage.Lane, exp.lane)
		}
		if len(stage.Dependencies) != len(exp.deps) {
			t.Fatalf("unexpected dependency count for %s: got %d want %d", stage.Name, len(stage.Dependencies), len(exp.deps))
		}
		for j, dep := range exp.deps {
			if stage.Dependencies[j] != dep {
				t.Fatalf("unexpected dependency %d for %s: got %s want %s", j, stage.Name, stage.Dependencies[j], dep)
			}
		}
	}
}

// TestPlannerAnnotatesModsMetadataFromAdvisor verifies that metadata from the
// knowledge base advisor is propagated into the Mods planning stages.
func TestPlannerAnnotatesModsMetadataFromAdvisor(t *testing.T) {
	advisor := &stubAdvisor{
		advice: mods.Advice{
			Plan: mods.AdvicePlan{
				SelectedRecipes: []string{"recipe.alpha", "recipe.beta"},
				ParallelStages:  []string{mods.StageNameORWApply, mods.StageNameORWGenerate},
				HumanGate:       true,
				Summary:         "expect human review after LLM execution",
			},
			Human: mods.AdviceHuman{
				Required:  true,
				Playbooks: []string{"playbook.mods.review"},
			},
			Recommendations: []mods.AdviceRecommendation{
				{Source: "knowledge-base", Message: "Apply recipe.alpha first", Confidence: 0.91},
				{Source: "knowledge-base", Message: "Queue review if diff size > 500", Confidence: 1.4},
				{Source: "knowledge-base", Message: " ", Confidence: -0.5},
			},
		},
	}

	planner := mods.NewPlanner(mods.Options{Advisor: advisor})

	stages, err := planner.Plan(context.Background(), mods.PlanInput{
        Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-knowledge"},
	})
	if err != nil {
		t.Fatalf("unexpected error planning mods stages: %v", err)
	}
	if !advisor.called {
		t.Fatalf("expected planner to invoke advisor")
	}

	planStage := findStage(t, stages, mods.StageNamePlan)
	if planStage.Metadata.Mods == nil || planStage.Metadata.Mods.Plan == nil {
		t.Fatalf("expected mods plan metadata present, got %#v", planStage.Metadata)
	}
	planMeta := planStage.Metadata.Mods.Plan
	if planMeta.HumanGate != true {
		t.Fatalf("expected human gate suggestion true, got %v", planMeta.HumanGate)
	}
	if len(planMeta.SelectedRecipes) != 2 {
		t.Fatalf("expected selected recipes recorded, got %#v", planMeta.SelectedRecipes)
	}
	if len(planMeta.ParallelStages) != 2 {
		t.Fatalf("expected parallel stages recorded, got %#v", planMeta.ParallelStages)
	}
	if planMeta.Summary == "" {
		t.Fatalf("expected summary present")
	}
	if len(planStage.Metadata.Mods.Recommendations) != 2 {
		t.Fatalf("expected recommendations recorded, got %#v", planStage.Metadata.Mods.Recommendations)
	}
	recs := planStage.Metadata.Mods.Recommendations
	if recs[1].Confidence != 1 {
		t.Fatalf("expected confidence clamped to 1, got %#v", recs[1].Confidence)
	}

	humanStage := findStage(t, stages, mods.StageNameHuman)
	if humanStage.Metadata.Mods == nil || humanStage.Metadata.Mods.Human == nil {
		t.Fatalf("expected human metadata present, got %#v", humanStage.Metadata)
	}
	humanMeta := humanStage.Metadata.Mods.Human
	if !humanMeta.Required {
		t.Fatalf("expected human stage required")
	}
	if len(humanMeta.Playbooks) != 1 {
		t.Fatalf("expected human playbook recorded, got %#v", humanMeta.Playbooks)
	}
}

// TestPlannerFallsBackWhenAdvisorErrors ensures planner metadata remains stable
// when the advisor fails.
func TestPlannerFallsBackWhenAdvisorErrors(t *testing.T) {
	errAdvisor := &stubAdvisor{err: errors.New("advisor unavailable")}
	planner := mods.NewPlanner(mods.Options{Advisor: errAdvisor})

	stages, err := planner.Plan(context.Background(), mods.PlanInput{
        Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-fallback"},
	})
	if err != nil {
		t.Fatalf("unexpected error planning mods stages: %v", err)
	}
	if !errAdvisor.called {
		t.Fatalf("expected planner to invoke advisor even when it errors")
	}

	planStage := findStage(t, stages, mods.StageNamePlan)
	if planStage.Metadata.Mods != nil {
		t.Fatalf("expected no mods metadata on failure, got %#v", planStage.Metadata.Mods)
	}
}

func TestPlannerExposesExecutionHints(t *testing.T) {
	options := mods.Options{}
	setModsOptionsHints(t, &options, 90*time.Second, 3)
	planner := mods.NewPlanner(options)

	stages, err := planner.Plan(context.Background(), mods.PlanInput{
        Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-hints"},
	})
	if err != nil {
		t.Fatalf("unexpected error planning mods stages: %v", err)
	}

	planStage := findStage(t, stages, mods.StageNamePlan)
	if planStage.Metadata.Mods == nil {
		t.Fatalf("expected mods metadata present on plan stage")
	}
	if planStage.Metadata.Mods.Plan == nil {
		t.Fatalf("expected plan metadata present on plan stage")
	}
	planMeta := *planStage.Metadata.Mods.Plan
	if len(planMeta.ParallelStages) == 0 {
		t.Fatalf("expected parallel stages recorded in plan metadata")
	}
	planValue := reflect.ValueOf(planMeta)
	planTimeoutField := planValue.FieldByName("PlanTimeout")
	if !planTimeoutField.IsValid() {
		t.Fatalf("plan metadata missing PlanTimeout field: %#v", planMeta)
	}
	if planTimeoutField.String() != "1m30s" {
		t.Fatalf("expected plan timeout 1m30s, got %q", planTimeoutField.String())
	}
	maxParallelField := planValue.FieldByName("MaxParallel")
	if !maxParallelField.IsValid() {
		t.Fatalf("plan metadata missing MaxParallel field: %#v", planMeta)
	}
	if int(maxParallelField.Int()) != 3 {
		t.Fatalf("expected max parallel 3, got %d", maxParallelField.Int())
	}
}

func TestPlannerIntegratesKnowledgeBaseAdvisor(t *testing.T) {
	dir := t.TempDir()
	fixture := `{
		"schema_version": "2025-09-27.1",
		"incidents": [
			{
				"id": "lint-failure",
				"errors": ["lint failed", "npm ERR! lint script"],
				"recipes": ["recipe.npm.lint"],
				"summary": "Run npm run lint with fixes",
				"human_gate": true,
				"playbooks": ["mods.npm.lint"],
				"recommendations": [
					{"source": "knowledge-base", "message": "Run npm run lint -- --fix", "confidence": 0.9}
				]
			}
		]
	}`
	path := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatalf("write catalog fixture: %v", err)
	}
	catalog, err := knowledgebase.LoadCatalogFile(path)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	advisor, err := knowledgebase.NewAdvisor(knowledgebase.Options{Catalog: catalog})
	if err != nil {
		t.Fatalf("new advisor: %v", err)
	}
	planner := mods.NewPlanner(mods.Options{Advisor: advisor})

        ticket := contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "KB-1", Manifest: contracts.ManifestReference{Name: "repo", Version: "1.0.0"}}
	stages, err := planner.Plan(context.Background(), mods.PlanInput{
		Ticket:  ticket,
		Signals: mods.AdviceSignals{Errors: []string{"npm ERR! lint script failed"}},
	})
	if err != nil {
		t.Fatalf("plan with knowledge base: %v", err)
	}
	planStage := findStage(t, stages, mods.StageNamePlan)
	if planStage.Metadata.Mods == nil || planStage.Metadata.Mods.Plan == nil {
		t.Fatalf("expected plan metadata present: %#v", planStage.Metadata)
	}
	if len(planStage.Metadata.Mods.Plan.SelectedRecipes) == 0 || planStage.Metadata.Mods.Plan.SelectedRecipes[0] != "recipe.npm.lint" {
		t.Fatalf("expected lint recipe recorded, got %#v", planStage.Metadata.Mods.Plan.SelectedRecipes)
	}
	if !planStage.Metadata.Mods.Plan.HumanGate {
		t.Fatalf("expected plan metadata to capture human gate")
	}
	humanStage := findStage(t, stages, mods.StageNameHuman)
	if humanStage.Metadata.Mods == nil || humanStage.Metadata.Mods.Human == nil {
		t.Fatalf("expected human metadata present on human stage")
	}
	if len(humanStage.Metadata.Mods.Human.Playbooks) == 0 || humanStage.Metadata.Mods.Human.Playbooks[0] != "mods.npm.lint" {
		t.Fatalf("expected human playbook from knowledge base, got %#v", humanStage.Metadata.Mods.Human.Playbooks)
	}
}

func setModsOptionsHints(t *testing.T, opts *mods.Options, planTimeout time.Duration, maxParallel int) {
	t.Helper()
	val := reflect.ValueOf(opts).Elem()
	planTimeoutField := val.FieldByName("PlanTimeout")
	if !planTimeoutField.IsValid() {
		t.Fatalf("mods.Options missing PlanTimeout field: %#v", opts)
	}
	if !planTimeoutField.CanSet() {
		t.Fatalf("mods.Options PlanTimeout not settable")
	}
	planTimeoutField.Set(reflect.ValueOf(planTimeout))

	maxParallelField := val.FieldByName("MaxParallel")
	if !maxParallelField.IsValid() {
		t.Fatalf("mods.Options missing MaxParallel field: %#v", opts)
	}
	if !maxParallelField.CanSet() {
		t.Fatalf("mods.Options MaxParallel not settable")
	}
	maxParallelField.SetInt(int64(maxParallel))

	for field, value := range map[string]string{
		"PlanLane":        "mods-plan",
		"OpenRewriteLane": "mods-java",
		"LLMPlanLane":     "mods-llm",
		"LLMExecLane":     "mods-llm",
		"HumanLane":       "mods-human",
	} {
		laneField := val.FieldByName(field)
		if laneField.IsValid() && laneField.CanSet() && laneField.Kind() == reflect.String {
			laneField.SetString(value)
		}
	}
}

// findStage locates a stage by name inside the Mods planner output during tests.
func findStage(t *testing.T, stages []mods.Stage, name string) mods.Stage {
	t.Helper()
	for _, stage := range stages {
		if stage.Name == name {
			return stage
		}
	}
	t.Fatalf("stage %s not found", name)
	return mods.Stage{}
}

// stubAdvisor implements mods.Advisor for tests.
type stubAdvisor struct {
	advice mods.Advice
	err    error
	called bool
}

// Advise returns the stubbed advisor response for tests.
func (s *stubAdvisor) Advise(ctx context.Context, req mods.AdviceRequest) (mods.Advice, error) {
	s.called = true
	if s.err != nil {
		return mods.Advice{}, s.err
	}
	return s.advice, nil
}
