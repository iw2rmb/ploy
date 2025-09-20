package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type captureReporter struct{ events []Event }

func (c *captureReporter) Report(ctx context.Context, ev Event) error {
	c.events = append(c.events, ev)
	return nil
}

func TestRunnerEmitsCloneAndBranchEvents(t *testing.T) {
	cfg := &ModConfig{
		ID:         "wf-ev1",
		TargetRepo: "https://example.com/repo.git",
		BaseRef:    "main",
		// Minimal placeholder step that will error after clone/branch, allowing us to observe events
		Steps: []ModStep{{Type: "recipe", ID: "noop"}},
	}
	work := t.TempDir()
	integrations := NewModIntegrationsWithTestMode("http://localhost:8080/v1", work, true)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("runner create: %v", err)
	}
	cap := &captureReporter{}
	runner.SetEventReporter(cap)

	// Run (expected to fail later due to no changes), but should emit early events
	_, _ = runner.Run(context.Background())

	if len(cap.events) == 0 {
		t.Fatalf("no events captured")
	}
	// Expect clone and create-branch events among the first emissions
	foundClone, foundBranch := false, false
	for _, ev := range cap.events {
		if ev.Step == "clone" {
			foundClone = true
		}
		if ev.Step == "create-branch" {
			foundBranch = true
		}
	}
	if !foundClone || !foundBranch {
		t.Fatalf("expected clone and create-branch events; got: %+v", cap.events)
	}
}

func TestRunnerEmitsApplyEvent(t *testing.T) {
	// Minimal config with one orw-apply step
	cfg := &ModConfig{
		ID:         "wf-ev2",
		TargetRepo: "https://example.com/repo.git",
		BaseRef:    "main",
		Steps:      []ModStep{{Type: "orw-apply", ID: "opt1", Recipes: []RecipeEntry{recipeEntry("org.openrewrite.java.migrate.Java11to17", "org.openrewrite.recipe", "rewrite-migrate-java", "3.17.0")}}},
	}
	work := t.TempDir()
	// Provide template expected by runner
	jobsDir := filepath.Join(work, "roadmap", "mods", "jobs")
	_ = os.MkdirAll(jobsDir, 0755)
	_ = os.WriteFile(filepath.Join(jobsDir, "orw_apply.hcl"), []byte("job \"orw-apply\" {}"), 0644)

	integrations := NewModIntegrationsWithTestMode("http://localhost:8080/v1", work, true)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("runner create: %v", err)
	}
	cap := &captureReporter{}
	runner.SetEventReporter(cap)

	// Stub out job submission to return quickly
	oldSubmit := submitAndWaitTerminal
	submitAndWaitTerminal = func(jobPath string, timeout time.Duration) error { return nil }
	defer func() { submitAndWaitTerminal = oldSubmit }()

	// Run (will fail later due to diff/build validations), but should emit apply event before
	_, _ = runner.Run(context.Background())

	foundApplyPhase := false
	for _, ev := range cap.events {
		if ev.Phase == "apply" {
			foundApplyPhase = true
			break
		}
	}
	if !foundApplyPhase {
		t.Fatalf("expected apply event; got: %+v", cap.events)
	}
}

func TestRunnerEmitsApplyErrorEvent(t *testing.T) {
	cfg := &ModConfig{
		ID:         "wf-ev3",
		TargetRepo: "https://example.com/repo.git",
		BaseRef:    "main",
		Steps:      []ModStep{{Type: "orw-apply", ID: "opt1", Recipes: []RecipeEntry{recipeEntry("org.openrewrite.java.migrate.Java11to17", "org.openrewrite.recipe", "rewrite-migrate-java", "3.17.0")}}},
	}
	work := t.TempDir()
	// Provide template expected by runner
	jobsDir := filepath.Join(work, "roadmap", "mods", "jobs")
	_ = os.MkdirAll(jobsDir, 0755)
	_ = os.WriteFile(filepath.Join(jobsDir, "orw_apply.hcl"), []byte("job \"orw-apply\" {}"), 0644)

	integrations := NewModIntegrationsWithTestMode("http://localhost:8080/v1", work, true)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("runner create: %v", err)
	}
	cap := &captureReporter{}
	runner.SetEventReporter(cap)

	// Force orw-apply submission to return error
	oldSubmit := submitAndWaitTerminal
	submitAndWaitTerminal = func(jobPath string, timeout time.Duration) error { return fmt.Errorf("simulated failure") }
	defer func() { submitAndWaitTerminal = oldSubmit }()

	_, _ = runner.Run(context.Background())

	foundApplyPhase := false
	for _, ev := range cap.events {
		if ev.Phase == "apply" {
			foundApplyPhase = true
			break
		}
	}
	if !foundApplyPhase {
		t.Fatalf("expected apply-phase events; got: %+v", cap.events)
	}
}
