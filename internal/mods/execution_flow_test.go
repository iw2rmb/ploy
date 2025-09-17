package mods

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type noopHCLSubmitter struct{}

func (noopHCLSubmitter) Validate(string) error              { return nil }
func (noopHCLSubmitter) Submit(string, time.Duration) error { return nil }
func (noopHCLSubmitter) SubmitCtx(context.Context, string, time.Duration) error {
	return nil
}

func findSubmitted(t *testing.T, dir string, suffix string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*"+suffix))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected file ending with %s under %s", suffix, dir)
	}
	return matches[0]
}

func TestExecuteWithPlanExecLLM(t *testing.T) {
	t.Setenv("MODS_SUBMIT", "0")
	workspace := t.TempDir()
	runner := &ModRunner{
		workspaceDir: workspace,
		config:       &ModConfig{ID: "cfg-id"},
		hcl:          noopHCLSubmitter{},
	}

	plan := map[string]any{
		"plan_id": "p-1",
		"options": []map[string]any{{"id": "llm-step", "type": string(StepTypeLLMExec)}},
	}
	planBytes, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}

	if err := executeWithPlan(runner, planBytes, false, true, false, false); err != nil {
		t.Fatalf("executeWithPlan execLLM: %v", err)
	}

	llmDir := filepath.Join(workspace, string(StepTypeLLMExec), "llm-step")
	submitted := findSubmitted(t, llmDir, ".submitted.hcl")
	if info, err := os.Stat(submitted); err != nil || info.IsDir() {
		t.Fatalf("invalid submitted file: %s (%v)", submitted, err)
	}
}

func TestExecuteWithPlanExecORW(t *testing.T) {
	t.Setenv("MODS_SUBMIT", "0")
	t.Setenv("MOD_ID", "mod-99")
	t.Setenv("PLOY_CONTROLLER", "https://controller.dev/v1")

	workspace := t.TempDir()
	runner := &ModRunner{
		workspaceDir: workspace,
		config:       &ModConfig{},
		hcl:          noopHCLSubmitter{},
	}

	plan := map[string]any{
		"plan_id": "p-2",
		"options": []map[string]any{{"id": "orw-option", "type": string(StepTypeORWGen)}},
	}
	planBytes, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}

	if err := executeWithPlan(runner, planBytes, false, false, true, false); err != nil {
		t.Fatalf("executeWithPlan execORW: %v", err)
	}

	orwDir := filepath.Join(workspace, string(StepTypeORWApply), "orw-option")
	submitted := findSubmitted(t, orwDir, ".submitted.hcl")
	if info, err := os.Stat(submitted); err != nil || info.IsDir() {
		t.Fatalf("invalid ORW submitted file: %s (%v)", submitted, err)
	}
}
