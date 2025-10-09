//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
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
		GridOutcomes: map[string][]runner.StageOutcome{
			"build-gate": {
				{
					Status:    runner.StageStatusFailed,
					Retryable: true,
					Message:   "compile failed: missing symbol",
				},
				{
					Status:    runner.StageStatusFailed,
					Retryable: true,
					Message:   "compile failed: missing symbol",
				},
			},
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
		GridOutcomes: map[string][]runner.StageOutcome{
			"build-gate": {
				{
					Status:    runner.StageStatusFailed,
					Retryable: true,
					Message:   "tests failed: multiple regressions",
				},
				{
					Status:    runner.StageStatusFailed,
					Retryable: true,
					Message:   "tests failed: multiple regressions",
				},
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

func assertStageSet(t *testing.T, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("stage count mismatch: got %d want %d (%v vs %v)", len(actual), len(expected), actual, expected)
	}
	unmatched := make(map[string]int, len(actual))
	for _, name := range actual {
		unmatched[name]++
	}
	for _, name := range expected {
		if unmatched[name] == 0 {
			t.Fatalf("missing stage %s in %v", name, actual)
		}
		unmatched[name]--
		if unmatched[name] == 0 {
			delete(unmatched, name)
		}
	}
	if len(unmatched) != 0 {
		t.Fatalf("unexpected stages present: %v", unmatched)
	}
}

func containsHealing(names []string) bool {
	for _, name := range names {
		if strings.Contains(name, "#heal") {
			return true
		}
	}
	return false
}

func idx(target string, values []string) int {
	return scenarioIndex(values, target)
}

func containsValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// TestModsScenariosLiveGrid executes configured Mods scenarios against live Grid when configuration is present.
func TestModsScenariosLiveGrid(t *testing.T) {
	cfg := LoadConfig()
	if cfg.SkipReason != "" {
		t.Skipf("live grid config missing: %s", cfg.SkipReason)
	}

	ids := liveGridScenarioIDs()
	if len(ids) == 0 {
		t.Skip("no live grid scenarios requested")
	}

	for _, id := range ids {
		scenario := mustScenario(t, id)
		t.Run(id, func(t *testing.T) {
			if err := runScenarioLive(t, cfg, scenario); err != nil {
				t.Fatalf("mods live grid scenario failed: %v", err)
			}
		})
	}
}

func TestLiveGridScenarioIDsDefault(t *testing.T) {
	if ids := liveGridScenarioIDs(); !reflect.DeepEqual(ids, []string{"simple-openrewrite"}) {
		t.Fatalf("unexpected live grid scenario ids: %v", ids)
	}
}

func TestLiveGridScenarioIDsFromEnv(t *testing.T) {
	t.Setenv("PLOY_E2E_LIVE_SCENARIOS", "parallel-healing-options, buildgate-self-heal ,simple-openrewrite")
	want := []string{"buildgate-self-heal", "parallel-healing-options", "simple-openrewrite"}
	got := liveGridScenarioIDs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected live grid scenario ids: got %v want %v", got, want)
	}
}

// runScenarioLive shells out to the CLI to execute a Mods scenario end-to-end on Grid.
func runScenarioLive(t *testing.T, cfg Config, scenario Scenario) error {
	t.Helper()
	binary := buildPloyBinary(t)
	repoURL := strings.TrimSpace(cfg.RepoOverride)
	if repoURL == "" {
		repoURL = strings.TrimSpace(scenario.RepoURL)
	}
	if repoURL == "" {
		return fmt.Errorf("scenario %s missing repo url", scenario.ID)
	}
	baseRef := strings.TrimSpace(scenario.BaseRef)
	if baseRef == "" {
		baseRef = "main"
	}
	targetRef := cfg.TargetRef(scenario.ID)
	workspaceHint := "mods/java"

	args := []string{
		"mod", "run",
		"--tenant", cfg.Tenant,
		"--ticket", cfg.TicketID(scenario.ID),
		"--repo-url", repoURL,
		"--repo-base-ref", baseRef,
		"--repo-target-ref", targetRef,
		"--repo-workspace-hint", workspaceHint,
	}

	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("determine repo root: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = rootDir

	envVars := map[string]string{
		"GRID_API_KEY":           cfg.GridAPIKey,
		"GRID_ID":                cfg.GridID,
		"PLOY_E2E_TENANT":        cfg.Tenant,
		"PLOY_E2E_TICKET_PREFIX": cfg.TicketPrefix,
		"PLOY_E2E_REPO_OVERRIDE": cfg.RepoOverride,
	}
	if cfg.BeaconURL != "" {
		envVars["GRID_CLIENT_BEACON_URL"] = cfg.BeaconURL
	}
	cmd.Env = ensureEnv(os.Environ(), envVars)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("mods live grid scenario timed out: %w", err)
		}
		return fmt.Errorf("mods live grid scenario error: %w\n%s", err, output.String())
	}

	return nil
}

// buildPloyBinary compiles the CLI into a temporary binary for the live Grid scenario run.
func buildPloyBinary(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "ploy-e2e")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/ploy")
	cmd.Env = os.Environ()
	cmd.Dir = projectRootDir(t)
	if result, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build ploy cli: %v\n%s", err, string(result))
	}
	return out
}

// projectRootDir resolves the repository root for invoking the CLI during tests.
func projectRootDir(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}
	return root
}

// ensureEnv merges override variables into the base environment without mutating the input slice.
func ensureEnv(base []string, overrides map[string]string) []string {
	result := make([]string, len(base))
	copy(result, base)
	for key, value := range overrides {
		if strings.TrimSpace(value) == "" {
			continue
		}
		result = upsertEnv(result, key, value)
	}
	return result
}

// upsertEnv inserts or replaces a single environment variable assignment.
func upsertEnv(env []string, key, value string) []string {
	needle := key + "="
	for idx, entry := range env {
		if strings.HasPrefix(entry, needle) {
			env[idx] = needle + value
			return env
		}
	}
	return append(env, needle+value)
}

// liveGridScenarioIDs resolves the list of scenario identifiers to execute against live Grid.
func liveGridScenarioIDs() []string {
	raw := strings.TrimSpace(os.Getenv("PLOY_E2E_LIVE_SCENARIOS"))
	if raw == "" {
		return []string{"simple-openrewrite"}
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	var ids []string
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if _, known := scenarioRegistry[id]; !known {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return []string{"simple-openrewrite"}
	}
	sort.Strings(ids)
	return ids
}
