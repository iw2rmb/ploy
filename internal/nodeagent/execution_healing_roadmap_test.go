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
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
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

	// Mock gate: always fails.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: false},
				},
				LogsText: "[ERROR] Build failure\n",
			}, nil
		},
	}

	// Mock container runtime that writes codex-last.txt with bug_summary to /out
	// when the router image runs, and tracks that the router was invoked.
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			if strings.Contains(spec.Image, "router") {
				routerRan = true
				// Write router output into the mounted /out directory so parseBugSummary can read it.
				for _, m := range spec.Mounts {
					if m.Target == "/out" {
						_ = os.WriteFile(filepath.Join(m.Source, "codex-last.txt"),
							[]byte(`{"bug_summary":"`+wantBugSummary+`"}`+"\n"), 0o644)
					}
				}
			}
			return step.ContainerHandle{ID: "mock-" + spec.Image}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

	workspace, err := os.MkdirTemp("", "ploy-test-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)

	outDir, err := os.MkdirTemp("", "ploy-test-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outDir)

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

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
				Mod: HealingMod{
					Image: contracts.ModImage{Universal: "test/healer:latest"},
				},
			},
			Router: &RouterConfig{
				Image: contracts.ModImage{Universal: "test/router:latest"},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Test",
		Image: "test/main:latest",
		Gate:  &contracts.StepGateSpec{Enabled: true},
	}

	// The gate fails, healing runs but gate still fails → ErrBuildGateFailed.
	// We mainly care that the router runs and bug_summary is set.
	initialGate, _, _, _ := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	// Router should have been invoked.
	if !routerRan {
		t.Error("expected router container to run after gate failure")
	}

	// bug_summary should be set on initialGate metadata.
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

	// Mock gate: passes.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			if strings.Contains(spec.Image, "router") {
				routerRan = true
			}
			return step.ContainerHandle{ID: "mock"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return nil, nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	workspace, _ := os.MkdirTemp("", "ploy-test-ws-*")
	defer os.RemoveAll(workspace)

	outDir, _ := os.MkdirTemp("", "ploy-test-out-*")
	defer os.RemoveAll(outDir)

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

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
				Mod: HealingMod{
					Image: contracts.ModImage{Universal: "test/healer:latest"},
				},
			},
			Router: &RouterConfig{
				Image: contracts.ModImage{Universal: "test/router:latest"},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Test",
		Image: "test/main:latest",
		Gate:  &contracts.StepGateSpec{Enabled: true},
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

	// Mock gate: fails first, passes second.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			if gateCallCount == 1 {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] Build failure\n",
				}, nil
			}
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
			}, nil
		},
	}

	// Track the outDir used by the healing container so we can pre-write
	// codex-last.txt with an action_summary.
	var healerOutDir string
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			return step.ContainerHandle{ID: "mock-container"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

	workspace, err := os.MkdirTemp("", "ploy-test-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)

	outDir, err := os.MkdirTemp("", "ploy-test-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outDir)
	healerOutDir = outDir

	// Pre-write codex-last.txt with action_summary to the outDir.
	// In real execution, the container writes this; in tests, we pre-populate it.
	_ = os.WriteFile(filepath.Join(healerOutDir, "codex-last.txt"),
		[]byte(`{"action_summary":"Added missing import for java.util.List"}`+"\n"), 0o644)

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

	req := StartRunRequest{
		RunID:     types.RunID("test-action-summary"),
		JobID:     types.JobID("test-job-action-summary"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod: HealingMod{
					Image: contracts.ModImage{Universal: "test/healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Test",
		Image: "test/main:latest",
		Gate:  &contracts.StepGateSpec{Enabled: true},
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
			},
		},
	}

	// Gate fails → healing runs (codex-last.txt is pre-populated) → re-gate passes.
	result, execErr := rc.executeWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0,
	)

	if execErr != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", execErr)
	}

	// Verify action_summary is propagated through executionResult.
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
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			if gateCallCount == 1 {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] Build failure\n",
				}, nil
			}
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			return step.ContainerHandle{ID: "mock-container"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return nil, nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	workspace, _ := os.MkdirTemp("", "ploy-test-ws-*")
	defer os.RemoveAll(workspace)

	outDir, _ := os.MkdirTemp("", "ploy-test-out-*")
	defer os.RemoveAll(outDir)

	// No codex-last.txt written.

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

	req := StartRunRequest{
		RunID:     types.RunID("test-no-summary"),
		JobID:     types.JobID("test-job-no-summary"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod: HealingMod{
					Image: contracts.ModImage{Universal: "test/healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Test",
		Image: "test/main:latest",
		Gate:  &contracts.StepGateSpec{Enabled: true},
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
			},
		},
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
// creates the expected /in artifacts: build-gate.log, build-gate-iteration-N.log,
// healing-iteration-N.log, and healing-log.md.
func TestHealingLog_ArtifactsCreatedWithCorrectNames(t *testing.T) {
	gateCallCount := 0

	// Mock gate: always fails (exhausts retries).
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: false},
				},
				LogsText: fmt.Sprintf("[ERROR] Failure iteration %d\n", gateCallCount),
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			return step.ContainerHandle{ID: "mock-container"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("container logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

	workspace, err := os.MkdirTemp("", "ploy-test-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)

	outDir, err := os.MkdirTemp("", "ploy-test-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outDir)

	// Pre-write codex.log to outDir (simulates healing agent output).
	_ = os.WriteFile(filepath.Join(outDir, "codex.log"), []byte("healing agent output\n"), 0o644)

	// Pre-write codex-last.txt with action_summary.
	_ = os.WriteFile(filepath.Join(outDir, "codex-last.txt"),
		[]byte(`{"action_summary":"Fixed missing semicolon"}`+"\n"), 0o644)

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

	req := StartRunRequest{
		RunID:     types.RunID("test-healing-log"),
		JobID:     types.JobID("test-job-healing-log"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 2,
				Mod: HealingMod{
					Image: contracts.ModImage{Universal: "test/healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Test",
		Image: "test/main:latest",
		Gate:  &contracts.StepGateSpec{Enabled: true},
	}

	// Run: gate fails, 2 healing attempts, all fail → ErrBuildGateFailed.
	_, _, _, _ = rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	// Verify /in directory was created.
	if inDir == "" {
		t.Fatal("/in directory not created")
	}
	defer os.RemoveAll(inDir)

	// Verify build-gate.log exists.
	assertFileExists(t, filepath.Join(inDir, "build-gate.log"))

	// Verify per-iteration gate logs.
	assertFileExists(t, filepath.Join(inDir, "build-gate-iteration-1.log"))
	assertFileExists(t, filepath.Join(inDir, "build-gate-iteration-2.log"))

	// Verify per-iteration healing logs (copied from codex.log).
	assertFileExists(t, filepath.Join(inDir, "healing-iteration-1.log"))
	assertFileExists(t, filepath.Join(inDir, "healing-iteration-2.log"))

	// Verify healing-log.md exists and has correct content.
	assertFileExists(t, filepath.Join(inDir, "healing-log.md"))
}

// TestHealingLog_MarkdownFormatCorrect verifies that healing-log.md has the
// expected markdown structure with iteration headers, bug/action summaries,
// and log file references.
func TestHealingLog_MarkdownFormatCorrect(t *testing.T) {
	gateCallCount := 0

	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: false},
				},
				LogsText: "[ERROR] Build failure\n",
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			return step.ContainerHandle{ID: "mock-container"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return nil, nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	workspace, _ := os.MkdirTemp("", "ploy-test-ws-*")
	defer os.RemoveAll(workspace)

	outDir, _ := os.MkdirTemp("", "ploy-test-out-*")
	defer os.RemoveAll(outDir)

	// Pre-write codex-last.txt with action_summary.
	_ = os.WriteFile(filepath.Join(outDir, "codex-last.txt"),
		[]byte(`{"action_summary":"Fixed compilation error"}`+"\n"), 0o644)

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

	req := StartRunRequest{
		RunID:     types.RunID("test-healing-md"),
		JobID:     types.JobID("test-job-healing-md"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod: HealingMod{
					Image: contracts.ModImage{Universal: "test/healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Test",
		Image: "test/main:latest",
		Gate:  &contracts.StepGateSpec{Enabled: true},
	}

	_, _, _, _ = rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if inDir == "" {
		t.Fatal("/in directory not created")
	}
	defer os.RemoveAll(inDir)

	healingLogPath := filepath.Join(inDir, "healing-log.md")
	data, err := os.ReadFile(healingLogPath)
	if err != nil {
		t.Fatalf("failed to read healing-log.md: %v", err)
	}

	content := string(data)

	// Verify markdown structure.
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
	gateCallCount := 0
	const wantBugSummary = "javac: cannot find symbol FooBar"

	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: false},
				},
				LogsText: "[ERROR] Build failure\n",
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			if strings.Contains(spec.Image, "router") {
				for _, m := range spec.Mounts {
					if m.Target == "/out" {
						_ = os.WriteFile(filepath.Join(m.Source, "codex-last.txt"),
							[]byte(`{"bug_summary":"`+wantBugSummary+`"}`+"\n"), 0o644)
					}
				}
			}
			return step.ContainerHandle{ID: "mock-" + spec.Image}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return nil, nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	workspace, _ := os.MkdirTemp("", "ploy-test-ws-*")
	defer os.RemoveAll(workspace)

	outDir, _ := os.MkdirTemp("", "ploy-test-out-*")
	defer os.RemoveAll(outDir)

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

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
				Mod: HealingMod{
					Image: contracts.ModImage{Universal: "test/healer:latest"},
				},
			},
			Router: &RouterConfig{
				Image: contracts.ModImage{Universal: "test/router:latest"},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Test",
		Image: "test/main:latest",
		Gate:  &contracts.StepGateSpec{Enabled: true},
	}

	_, _, _, _ = rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if inDir == "" {
		t.Fatal("/in directory not created")
	}
	defer os.RemoveAll(inDir)

	// Verify healing-log.md exists. The bug_summary will be "N/A" since the
	// mock router doesn't actually write to its own temp outDir, but the
	// markdown format should still be correct.
	healingLogPath := filepath.Join(inDir, "healing-log.md")
	data, err := os.ReadFile(healingLogPath)
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

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", path)
	}
}
