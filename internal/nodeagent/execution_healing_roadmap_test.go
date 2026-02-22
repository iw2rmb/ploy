package nodeagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// ---------------------------------------------------------------------------
// TestRouter_* — exercises router execution path
// ---------------------------------------------------------------------------

// TestRouter_FailingGateTriggersRouterAndSetsBugSummary verifies that when the gate
// fails and a router is configured, the router runs and its bug_summary is set on
// the initialGate metadata.
func TestRouter_FailingGateTriggersRouterAndSetsBugSummary(t *testing.T) {
	var routerRan bool
	const wantBugSummary = "Missing semicolon on line 42 of Main.java"

	mc := noopContainer()
	mc.createFn = func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		if strings.Contains(spec.Image, "router") {
			routerRan = true
			for _, m := range spec.Mounts {
				if m.Target == "/out" {
					_ = os.WriteFile(filepath.Join(m.Source, "codex-last.txt"),
						[]byte(`{"bug_summary":"`+wantBugSummary+`"}`+"\n"), 0o644)
				}
			}
		}
		return step.ContainerHandle{ID: "mock-" + spec.Image}, nil
	}

	workspace, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(failingGate(), mc)
	rc := healingRC()

	req := StartRunRequest{
		RunID:     types.RunID("test-router"),
		JobID:     types.JobID("test-job-router"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     ModContainerSpec{Image: contracts.ModImage{Universal: "test/healer:latest"}},
			},
			Router: &ModContainerSpec{
				Image: contracts.ModImage{Universal: "test/router:latest"},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID: types.StepID(req.JobID), Name: "Test", Image: "test/main:latest",
		Gate: &contracts.StepGateSpec{Enabled: true},
	}

	initialGate, _, _, _ := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if !routerRan {
		t.Error("expected router container to run after gate failure")
	}
	if initialGate == nil || initialGate.Metadata == nil {
		t.Fatal("initialGate or its metadata is nil")
	}
	if initialGate.Metadata.BugSummary != wantBugSummary {
		t.Fatalf("bug_summary on initialGate = %q, want %q", initialGate.Metadata.BugSummary, wantBugSummary)
	}
}

// TestRouter_NotRunWhenGatePasses verifies that the router does NOT run when
// the gate passes (no gate failure → no router).
func TestRouter_NotRunWhenGatePasses(t *testing.T) {
	var routerRan bool

	mc := noopContainer()
	mc.createFn = func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		if strings.Contains(spec.Image, "router") {
			routerRan = true
		}
		return step.ContainerHandle{ID: "mock"}, nil
	}

	workspace, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(passingGate(), mc)
	rc := healingRC()

	req := StartRunRequest{
		RunID:     types.RunID("test-router-pass"),
		JobID:     types.JobID("test-job-router-pass"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     ModContainerSpec{Image: contracts.ModImage{Universal: "test/healer:latest"}},
			},
			Router: &ModContainerSpec{
				Image: contracts.ModImage{Universal: "test/router:latest"},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID: types.StepID(req.JobID), Name: "Test", Image: "test/main:latest",
		Gate: &contracts.StepGateSpec{Enabled: true},
	}

	_, _, _, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if gateErr != nil {
		t.Fatalf("expected gate to pass, got error: %v", gateErr)
	}
	if routerRan {
		t.Error("router should NOT run when gate passes")
	}
}

// ---------------------------------------------------------------------------
// TestHealingSummary_* — verifies action_summary persistence
// ---------------------------------------------------------------------------

// TestHealingSummary_ActionSummaryPropagatedViaResult verifies that when the
// healing container writes an action_summary to /out/codex-last.txt, the value
// is captured and propagated through executionResult.ActionSummary.
func TestHealingSummary_ActionSummaryPropagatedViaResult(t *testing.T) {
	gateCallCount := 0
	mockGate := &mockGateExecutor{
		executeFn: func(_ context.Context, _ *contracts.StepGateSpec, _ string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			if gateCallCount == 1 {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
					LogsText:     "[ERROR] Build failure\n",
				}, nil
			}
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}},
			}, nil
		},
	}

	workspace, outDir := healingDirs(t)
	_ = os.WriteFile(filepath.Join(outDir, "codex-last.txt"),
		[]byte(`{"action_summary":"Added missing import for java.util.List"}`+"\n"), 0o644)

	inDir := ""
	runner := healingRunner(mockGate, noopContainer())
	rc := healingRC()
	req := healingRequest("test-action-summary", "test-job-action-summary", 1, "test/healer:latest")
	req.Env = map[string]string{}
	manifest := contracts.StepManifest{
		ID: types.StepID(req.JobID), Name: "Test", Image: "test/main:latest",
		Gate: &contracts.StepGateSpec{Enabled: true},
		Inputs: []contracts.StepInput{{
			Name: "workspace", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite,
		}},
	}

	result, execErr := rc.executeWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0,
	)

	if execErr != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", execErr)
	}
	wantSummary := "Added missing import for java.util.List"
	if result.ActionSummary != wantSummary {
		t.Errorf("executionResult.ActionSummary = %q, want %q", result.ActionSummary, wantSummary)
	}
}

// TestHealingSummary_EmptyWhenNoCodexLastFile verifies that when the healing
// container does not write codex-last.txt, actionSummary is empty.
func TestHealingSummary_EmptyWhenNoCodexLastFile(t *testing.T) {
	gateCallCount := 0
	mockGate := &mockGateExecutor{
		executeFn: func(_ context.Context, _ *contracts.StepGateSpec, _ string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			if gateCallCount == 1 {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
					LogsText:     "[ERROR] Build failure\n",
				}, nil
			}
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}},
			}, nil
		},
	}

	workspace, outDir := healingDirs(t)
	// No codex-last.txt written.
	inDir := ""
	runner := healingRunner(mockGate, noopContainer())
	rc := healingRC()
	req := healingRequest("test-no-summary", "test-job-no-summary", 1, "test/healer:latest")
	req.Env = map[string]string{}
	manifest := contracts.StepManifest{
		ID: types.StepID(req.JobID), Name: "Test", Image: "test/main:latest",
		Gate: &contracts.StepGateSpec{Enabled: true},
		Inputs: []contracts.StepInput{{
			Name: "workspace", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite,
		}},
	}

	result, execErr := rc.executeWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0,
	)

	if execErr != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", execErr)
	}
	if result.ActionSummary != "" {
		t.Errorf("executionResult.ActionSummary = %q, want empty string", result.ActionSummary)
	}
}

// ---------------------------------------------------------------------------
// TestHealingLog_* — verifies artifact file names and markdown formatting
// ---------------------------------------------------------------------------

// TestHealingLog_ArtifactsCreatedWithCorrectNames verifies that the healing loop
// creates the expected /in artifacts.
func TestHealingLog_ArtifactsCreatedWithCorrectNames(t *testing.T) {
	gateCallCount := 0
	mockGate := &mockGateExecutor{
		executeFn: func(_ context.Context, _ *contracts.StepGateSpec, _ string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
				LogsText:     fmt.Sprintf("[ERROR] Failure iteration %d\n", gateCallCount),
			}, nil
		},
	}

	workspace, outDir := healingDirs(t)
	_ = os.WriteFile(filepath.Join(outDir, "codex.log"), []byte("healing agent output\n"), 0o644)
	_ = os.WriteFile(filepath.Join(outDir, "codex-last.txt"),
		[]byte(`{"action_summary":"Fixed missing semicolon"}`+"\n"), 0o644)

	inDir := ""
	runner := healingRunner(mockGate, noopContainer())
	rc := healingRC()
	req := healingRequest("test-healing-log", "test-job-healing-log", 2, "test/healer:latest")
	req.Env = map[string]string{}
	manifest := contracts.StepManifest{
		ID: types.StepID(req.JobID), Name: "Test", Image: "test/main:latest",
		Gate: &contracts.StepGateSpec{Enabled: true},
	}

	_, _, _, _ = rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if inDir == "" {
		t.Fatal("/in directory not created")
	}
	defer os.RemoveAll(inDir)

	assertFileExists(t, filepath.Join(inDir, "build-gate.log"))
	assertFileExists(t, filepath.Join(inDir, "build-gate-iteration-1.log"))
	assertFileExists(t, filepath.Join(inDir, "build-gate-iteration-2.log"))
	assertFileExists(t, filepath.Join(inDir, "healing-iteration-1.log"))
	assertFileExists(t, filepath.Join(inDir, "healing-iteration-2.log"))
	assertFileExists(t, filepath.Join(inDir, "healing-log.md"))
}

// TestHealingLog_MarkdownFormatCorrect verifies that healing-log.md has the
// expected markdown structure.
func TestHealingLog_MarkdownFormatCorrect(t *testing.T) {
	workspace, outDir := healingDirs(t)
	_ = os.WriteFile(filepath.Join(outDir, "codex-last.txt"),
		[]byte(`{"action_summary":"Fixed compilation error"}`+"\n"), 0o644)

	inDir := ""
	runner := healingRunner(failingGate(), noopContainer())
	rc := healingRC()
	req := healingRequest("test-healing-md", "test-job-healing-md", 1, "test/healer:latest")
	req.Env = map[string]string{}
	manifest := contracts.StepManifest{
		ID: types.StepID(req.JobID), Name: "Test", Image: "test/main:latest",
		Gate: &contracts.StepGateSpec{Enabled: true},
	}

	_, _, _, _ = rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if inDir == "" {
		t.Fatal("/in directory not created")
	}
	defer os.RemoveAll(inDir)

	data, err := os.ReadFile(filepath.Join(inDir, "healing-log.md"))
	if err != nil {
		t.Fatalf("failed to read healing-log.md: %v", err)
	}

	content := string(data)
	expectations := []string{
		"# Healing Log",
		"## Iteration 1",
		"- Bug Summary: N/A",
		"  Build Log: /in/build-gate-iteration-1.log",
		"- Healing Attempt: Fixed compilation error",
		"  Agent Log: /in/healing-iteration-1.log",
	}
	for _, expect := range expectations {
		if !strings.Contains(content, expect) {
			t.Errorf("healing-log.md missing expected content: %q\ngot:\n%s", expect, content)
		}
	}
}

// TestHealingLog_BugSummaryIncludedWhenRouterRuns verifies that when both router
// and healing are configured, the bug_summary from the router appears in
// healing-log.md.
func TestHealingLog_BugSummaryIncludedWhenRouterRuns(t *testing.T) {
	const wantBugSummary = "javac: cannot find symbol FooBar"

	mc := noopContainer()
	mc.createFn = func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		if strings.Contains(spec.Image, "router") {
			for _, m := range spec.Mounts {
				if m.Target == "/out" {
					_ = os.WriteFile(filepath.Join(m.Source, "codex-last.txt"),
						[]byte(`{"bug_summary":"`+wantBugSummary+`"}`+"\n"), 0o644)
				}
			}
		}
		return step.ContainerHandle{ID: "mock-" + spec.Image}, nil
	}

	workspace, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(failingGate(), mc)
	rc := healingRC()

	req := StartRunRequest{
		RunID:     types.RunID("test-bug-in-md"),
		JobID:     types.JobID("test-job-bug-in-md"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     ModContainerSpec{Image: contracts.ModImage{Universal: "test/healer:latest"}},
			},
			Router: &ModContainerSpec{
				Image: contracts.ModImage{Universal: "test/router:latest"},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID: types.StepID(req.JobID), Name: "Test", Image: "test/main:latest",
		Gate: &contracts.StepGateSpec{Enabled: true},
	}

	_, _, _, _ = rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if inDir == "" {
		t.Fatal("/in directory not created")
	}
	defer os.RemoveAll(inDir)

	data, err := os.ReadFile(filepath.Join(inDir, "healing-log.md"))
	if err != nil {
		t.Fatalf("failed to read healing-log.md: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "# Healing Log") {
		t.Error("healing-log.md missing '# Healing Log' header")
	}
	if !strings.Contains(content, "## Iteration 1") {
		t.Error("healing-log.md missing '## Iteration 1' header")
	}
	if !strings.Contains(content, "- Bug Summary: "+wantBugSummary) {
		t.Fatalf("healing-log.md missing expected bug summary %q\ngot:\n%s", wantBugSummary, content)
	}
}
