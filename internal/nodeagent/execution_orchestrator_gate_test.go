package nodeagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// Verify gate stats shape includes an explicit final_gate key when only a final
// gate run is present (no pre_gate/regates), and does not replace the root map.
func TestBuildGateStats_FinalOnlyShape(t *testing.T) {
	rc := &runController{cfg: Config{}}
	result := step.Result{
		BuildGate: &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}}},
		Timings:   step.StageTiming{BuildGateDuration: 123},
	}
	execRes := executionResult{}

	got := rc.buildGateStats(types.RunID("run-x"), types.JobID("stage-y"), result, execRes)
	if got == nil || got.FinalGate == nil {
		t.Fatalf("missing final_gate in gate stats")
	}
	if got.FinalGate.Passed != true {
		t.Fatalf("final_gate passed=%v, want true", got.FinalGate.Passed)
	}
}

// TestMergeExecutionResults_PreservesPreModGate verifies that when a pre-mod gate
// has already been recorded in the accumulator, merging a per-step execution
// result keeps the original PreGate and appends new ReGates in order.
func TestMergeExecutionResults_PreservesPreModGate(t *testing.T) {
	preModMeta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Tool: "pre-mod", Passed: true},
		},
	}
	preModGate := &gateRunMetadata{
		Metadata:   preModMeta,
		DurationMs: 100,
	}
	preReGate := gateRunMetadata{
		Metadata: &contracts.BuildGateStageMetadata{
			StaticChecks: []contracts.BuildGateStaticCheckReport{
				{Tool: "pre-regate", Passed: true},
			},
		},
		DurationMs: 200,
	}

	acc := executionResult{
		PreGate: preModGate,
		ReGates: []gateRunMetadata{preReGate},
	}

	stepPreGate := &gateRunMetadata{
		Metadata: &contracts.BuildGateStageMetadata{
			StaticChecks: []contracts.BuildGateStaticCheckReport{
				{Tool: "step-pre", Passed: false},
			},
		},
		DurationMs: 50,
	}
	stepReGate := gateRunMetadata{
		Metadata: &contracts.BuildGateStageMetadata{
			StaticChecks: []contracts.BuildGateStaticCheckReport{
				{Tool: "step-regate", Passed: true},
			},
		},
		DurationMs: 300,
	}

	next := executionResult{
		Result:  step.Result{ExitCode: 0},
		PreGate: stepPreGate,
		ReGates: []gateRunMetadata{stepReGate},
	}

	merged := mergeExecutionResults(acc, next)

	// PreGate should remain the pre-mod gate from the accumulator.
	if merged.PreGate != preModGate {
		t.Fatalf("merged.PreGate = %#v, want accumulator pre-mod gate %#v", merged.PreGate, preModGate)
	}

	// ReGates should contain accumulator re-gates followed by next re-gates.
	if len(merged.ReGates) != 2 {
		t.Fatalf("len(merged.ReGates) = %d, want 2", len(merged.ReGates))
	}
	if merged.ReGates[0] != preReGate {
		t.Errorf("merged.ReGates[0] = %#v, want preReGate %#v", merged.ReGates[0], preReGate)
	}
	if merged.ReGates[1] != stepReGate {
		t.Errorf("merged.ReGates[1] = %#v, want stepReGate %#v", merged.ReGates[1], stepReGate)
	}

	// Result should come from the next execution result.
	if merged.ExitCode != 0 {
		t.Errorf("merged.ExitCode = %d, want 0", merged.ExitCode)
	}
}

// TestBuildGateStats_PreGateFallbackToFinalGate verifies that when no post-mod gate
// (result.BuildGate) exists but a pre-mod gate was recorded, buildGateStats populates
// final_gate from the pre-mod gate. This ensures CLI/API gate summaries always have
// a final_gate to report on, even when no mods executed.
func TestBuildGateStats_PreGateFallbackToFinalGate(t *testing.T) {
	rc := &runController{cfg: Config{}}

	// Pre-mod gate only — simulates a run that terminated before any mod execution.
	preGateMeta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Tool: "maven", Passed: true},
		},
	}
	execRes := executionResult{
		PreGate: &gateRunMetadata{
			Metadata:   preGateMeta,
			DurationMs: 500,
		},
	}

	// No BuildGate in result (no mods executed).
	result := step.Result{}

	got := rc.buildGateStats(types.RunID("run-fallback"), types.JobID("stage-fallback"), result, execRes)
	if got == nil {
		t.Fatalf("expected gate stats, got nil")
	}
	if got.PreGate == nil {
		t.Fatalf("expected pre_gate in gate stats")
	}
	if got.FinalGate == nil {
		t.Fatalf("expected final_gate to be populated from pre-mod gate fallback")
	}

	// Verify final_gate content matches pre-mod gate.
	if got.FinalGate.Passed != true {
		t.Errorf("final_gate passed=%v, want true", got.FinalGate.Passed)
	}
	if got.FinalGate.DurationMs != int64(500) {
		t.Errorf("final_gate duration_ms=%d, want 500", got.FinalGate.DurationMs)
	}
}

// TestBuildGateStats_PostGateTakesPrecedence verifies that when both pre-mod gate
// and post-mod gate (result.BuildGate) exist, final_gate uses the post-mod gate,
// not the pre-mod gate fallback.
func TestBuildGateStats_PostGateTakesPrecedence(t *testing.T) {
	rc := &runController{cfg: Config{}}

	// Both pre-mod and post-mod gates present.
	preGateMeta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Tool: "maven", Passed: true},
		},
	}
	postGateMeta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Tool: "gradle", Passed: false},
		},
	}

	execRes := executionResult{
		PreGate: &gateRunMetadata{
			Metadata:   preGateMeta,
			DurationMs: 300,
		},
	}

	result := step.Result{
		BuildGate: postGateMeta,
		Timings:   step.StageTiming{BuildGateDuration: types.Duration(700 * time.Millisecond)},
	}

	got := rc.buildGateStats(types.RunID("run-precedence"), types.JobID("stage-precedence"), result, execRes)

	// Verify final_gate uses the post-mod gate (result.BuildGate), not the pre-mod fallback.
	if got == nil || got.FinalGate == nil {
		t.Fatalf("expected final_gate in gate stats")
	}

	// Post-mod gate had passed=false, duration=700ms.
	if got.FinalGate.Passed != false {
		t.Errorf("final_gate passed=%v, want false (from post-mod gate)", got.FinalGate.Passed)
	}
	if got.FinalGate.DurationMs != int64(700) {
		t.Errorf("final_gate duration_ms=%d, want 700 (from post-mod gate)", got.FinalGate.DurationMs)
	}
}

// TestPersistFirstGateFailureLog_UsesTrimmedFinding verifies that the first failing
// gate log persisted for healing prefers the trimmed LogFindings view over LogsText.
func TestPersistFirstGateFailureLog_UsesTrimmedFinding(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-trimmed-log")

	full := "[INFO] noise\n[ERROR] important failure\nstack\n"
	trimmed := "[ERROR] important failure\nstack\n"

	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Tool: "maven", Passed: false},
		},
		LogsText: full,
		LogFindings: []contracts.BuildGateLogFinding{
			{Severity: "error", Message: trimmed},
		},
	}

	rc.persistFirstGateFailureLog(runID, meta)

	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	runDir := filepath.Join(baseRoot, "ploy", "run", runID.String())
	logPath := filepath.Join(runDir, "build-gate-first.log")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read persisted gate log: %v", err)
	}

	got := string(data)
	if got != trimmed && got != trimmed+"\n" {
		t.Fatalf("persisted gate log = %q, want trimmed log %q", got, trimmed)
	}
}

// TestMergeExecutionResults_UsesNextPreGateWhenNoAccumulator verifies that when
// there is no pre-mod gate recorded yet, mergeExecutionResults falls back to
// the next execution's PreGate.
func TestMergeExecutionResults_UsesNextPreGateWhenNoAccumulator(t *testing.T) {
	nextPreGate := &gateRunMetadata{
		Metadata: &contracts.BuildGateStageMetadata{
			StaticChecks: []contracts.BuildGateStaticCheckReport{
				{Tool: "step-pre", Passed: true},
			},
		},
		DurationMs: 42,
	}

	acc := executionResult{}
	next := executionResult{
		Result:  step.Result{ExitCode: 0},
		PreGate: nextPreGate,
	}

	merged := mergeExecutionResults(acc, next)

	if merged.PreGate != nextPreGate {
		t.Fatalf("merged.PreGate = %#v, want nextPreGate %#v", merged.PreGate, nextPreGate)
	}
	if merged.ExitCode != 0 {
		t.Errorf("merged.ExitCode = %d, want 0", merged.ExitCode)
	}
}

// TestPersistGateStack_WritesStack verifies that persistGateStack writes the
// detected stack to a file under the run directory for later retrieval.
func TestPersistGateStack_WritesStack(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-stack-persist")

	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Language: "java", Tool: "maven", Passed: true},
		},
	}

	rc.persistGateStack(runID, meta)

	// Verify the stack file was created with the correct content.
	stackPath := filepath.Join(cacheHome, "ploy", "run", runID.String(), "build-gate-stack.txt")
	data, err := os.ReadFile(stackPath)
	if err != nil {
		t.Fatalf("failed to read persisted stack file: %v", err)
	}

	got := string(data)
	if got != "java-maven" {
		t.Errorf("persisted stack = %q, want %q", got, "java-maven")
	}
}

// TestPersistGateStack_Idempotent verifies that persistGateStack only writes
// the first detection and ignores subsequent calls.
func TestPersistGateStack_Idempotent(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-stack-idempotent")

	// First persist: Maven.
	metaMaven := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Language: "java", Tool: "maven", Passed: true},
		},
	}
	rc.persistGateStack(runID, metaMaven)

	// Second persist: Gradle (should be ignored).
	metaGradle := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Language: "java", Tool: "gradle", Passed: true},
		},
	}
	rc.persistGateStack(runID, metaGradle)

	// Verify the first stack is preserved.
	stackPath := filepath.Join(cacheHome, "ploy", "run", runID.String(), "build-gate-stack.txt")
	data, err := os.ReadFile(stackPath)
	if err != nil {
		t.Fatalf("failed to read persisted stack file: %v", err)
	}

	got := string(data)
	if got != "java-maven" {
		t.Errorf("persisted stack = %q, want first stack %q", got, "java-maven")
	}
}

// TestLoadPersistedStack_ReturnsStack verifies that loadPersistedStack reads
// the persisted stack from the run directory.
func TestLoadPersistedStack_ReturnsStack(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-stack-load")

	// Seed the stack file manually.
	runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}
	stackPath := filepath.Join(runDir, "build-gate-stack.txt")
	if err := os.WriteFile(stackPath, []byte("java-gradle"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	got := rc.loadPersistedStack(runID)
	if got != contracts.ModStackJavaGradle {
		t.Errorf("loadPersistedStack() = %q, want %q", got, contracts.ModStackJavaGradle)
	}
}

// TestLoadPersistedStack_DefaultsToUnknown verifies that loadPersistedStack
// returns ModStackUnknown when no stack file exists.
func TestLoadPersistedStack_DefaultsToUnknown(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-stack-missing")

	got := rc.loadPersistedStack(runID)
	if got != contracts.ModStackUnknown {
		t.Errorf("loadPersistedStack() = %q, want %q", got, contracts.ModStackUnknown)
	}
}

// TestPersistAndLoadGateStack_RoundTrip verifies the complete flow of persisting
// a stack during gate execution and loading it for mod/healing execution.
func TestPersistAndLoadGateStack_RoundTrip(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-stack-roundtrip")

	// Simulate gate execution result.
	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Language: "java", Tool: "gradle", Passed: false},
		},
	}

	// Persist during gate job.
	rc.persistGateStack(runID, meta)

	// Load during mod/healing job.
	got := rc.loadPersistedStack(runID)
	if got != contracts.ModStackJavaGradle {
		t.Errorf("round-trip stack = %q, want %q", got, contracts.ModStackJavaGradle)
	}
}

// TestBuildGateJobStats_IncludesJobMeta verifies that gate job stats embed
// JobMeta so that jobs.meta can carry structured gate metadata.
func TestBuildGateJobStats_IncludesJobMeta(t *testing.T) {
	t.Parallel()

	rc := &runController{cfg: Config{}}

	gateMeta := &contracts.BuildGateStageMetadata{
		LogDigest: testLogDigest(1),
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Tool: "maven", Passed: true},
		},
	}

	stats := rc.buildGateJobStats(gateMeta, 250*time.Millisecond)

	// RunStats is now json.RawMessage-backed. Decode to access job_meta.
	// Parse the stats as JSON and extract the job_meta field.
	var decoded struct {
		JobMeta *contracts.JobMeta `json:"job_meta"`
	}
	if err := json.Unmarshal(stats, &decoded); err != nil {
		t.Fatalf("failed to unmarshal stats: %v", err)
	}
	if decoded.JobMeta == nil {
		t.Fatalf("expected job_meta key in gate stats, got nil")
	}

	if decoded.JobMeta.Kind != contracts.JobKindGate {
		t.Fatalf("job_meta.Kind = %q, want %q", decoded.JobMeta.Kind, contracts.JobKindGate)
	}
	if decoded.JobMeta.Gate == nil || decoded.JobMeta.Gate.LogDigest != testLogDigest(1) {
		t.Fatalf("job_meta.Gate.LogDigest = %#v, want %q", decoded.JobMeta.Gate, testLogDigest(1))
	}
}

func TestRunRouterForGateFailure_SetsBugSummary(t *testing.T) {
	t.Parallel()

	rc := &runController{cfg: Config{ServerURL: "http://localhost:9999"}}

	workspace := t.TempDir()

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

	runner := step.Runner{Containers: mc}

	req := StartRunRequest{
		RunID:   types.RunID("run-router-gate"),
		JobID:   types.JobID("job-router-gate"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
	}

	typedOpts := RunOptions{
		Healing: &HealingConfig{
			Retries: 1,
			Mod: HealingMod{
				Image: contracts.ModImage{Universal: "test/healer:latest"},
			},
		},
		Router: &RouterConfig{
			Image: contracts.ModImage{Universal: "test/router:latest"},
		},
	}

	gateResult := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
		LogsText:     "[ERROR] build failed\n",
	}

	rc.runRouterForGateFailure(context.Background(), runner, req, typedOpts, workspace, gateResult)

	if gateResult.BugSummary != wantBugSummary {
		t.Fatalf("gateResult.BugSummary = %q, want %q", gateResult.BugSummary, wantBugSummary)
	}
}
