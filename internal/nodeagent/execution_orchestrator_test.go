package nodeagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// stubDiffGen is a minimal DiffGenerator used for testing uploadDiffForStep.
type stubDiffGen struct{}

func (stubDiffGen) Generate(_ context.Context, _ string) ([]byte, error) {
	return []byte("diff-bytes"), nil
}

// Verify gate stats shape includes an explicit final_gate key when only a final
// gate run is present (no pre_gate/regates), and does not replace the root map.
func TestBuildGateStats_FinalOnlyShape(t *testing.T) {
	rc := &runController{cfg: Config{}}
	result := step.Result{
		BuildGate: &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}}},
		Timings:   step.StageTiming{BuildGateDuration: 123},
	}
	execRes := executionResult{}

	got := rc.buildGateStats("run-x", "stage-y", result, execRes)

	if _, has := got["duration_ms"]; has {
		t.Fatalf("unexpected flat stats at root; want nested 'final_gate'")
	}
	fg, has := got["final_gate"]
	if !has {
		t.Fatalf("missing final_gate key in gate stats")
	}
	if m, ok := fg.(map[string]any); !ok || m["passed"] != true {
		t.Fatalf("final_gate stats malformed or missing passed=true: %#v", fg)
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
	if merged.Result.ExitCode != 0 {
		t.Errorf("merged.Result.ExitCode = %d, want 0", merged.Result.ExitCode)
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

	got := rc.buildGateStats("run-fallback", "stage-fallback", result, execRes)

	// Verify pre_gate is present.
	if _, hasPre := got["pre_gate"]; !hasPre {
		t.Fatalf("expected pre_gate in gate stats, got: %#v", got)
	}

	// Verify final_gate is populated from the pre-mod gate fallback.
	fg, hasFinal := got["final_gate"]
	if !hasFinal {
		t.Fatalf("expected final_gate to be populated from pre-mod gate fallback, got: %#v", got)
	}

	fgMap, ok := fg.(map[string]any)
	if !ok {
		t.Fatalf("final_gate has unexpected type %T", fg)
	}

	// Verify final_gate content matches pre-mod gate.
	if fgMap["passed"] != true {
		t.Errorf("final_gate passed=%v, want true", fgMap["passed"])
	}
	if fgMap["duration_ms"] != int64(500) {
		t.Errorf("final_gate duration_ms=%v, want 500", fgMap["duration_ms"])
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
		Timings:   step.StageTiming{BuildGateDuration: 700 * time.Millisecond},
	}

	got := rc.buildGateStats("run-precedence", "stage-precedence", result, execRes)

	// Verify final_gate uses the post-mod gate (result.BuildGate), not the pre-mod fallback.
	fg, hasFinal := got["final_gate"]
	if !hasFinal {
		t.Fatalf("expected final_gate in gate stats, got: %#v", got)
	}

	fgMap, ok := fg.(map[string]any)
	if !ok {
		t.Fatalf("final_gate has unexpected type %T", fg)
	}

	// Post-mod gate had passed=false, duration=700ms.
	if fgMap["passed"] != false {
		t.Errorf("final_gate passed=%v, want false (from post-mod gate)", fgMap["passed"])
	}
	if fgMap["duration_ms"] != int64(700) {
		t.Errorf("final_gate duration_ms=%v, want 700 (from post-mod gate)", fgMap["duration_ms"])
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
	if merged.Result.ExitCode != 0 {
		t.Errorf("merged.Result.ExitCode = %d, want 0", merged.Result.ExitCode)
	}
}

// TestUploadDiffForStep_TagsStepIndex verifies that uploadDiffForStep includes
// step_index both at the top level and inside the summary for proper ordering in multi-step runs.
func TestUploadDiffForStep_TagsStepIndex(t *testing.T) {
	// Provide a bearer token for createHTTPClient via the overridable path.
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "bearer-token")
	if err := os.WriteFile(tokenPath, []byte("test-token"), 0o644); err != nil {
		t.Fatalf("write bearer token: %v", err)
	}
	t.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tokenPath)

	// Capture the diff upload request.
	var (
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		defer r.Body.Close()

		if err := json.Unmarshal(body, &gotPayload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    "node-1",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	ctx := context.Background()
	stepIndex := 2

	rc.uploadDiffForStep(
		ctx,
		"run-123",
		"stage-abc",
		stubDiffGen{},
		"/unused/workspace",
		step.Result{},
		stepIndex,
	)

	if gotPayload == nil {
		t.Fatalf("no payload captured from diff upload")
	}

	// Verify request path uses node id and stage id.
	if gotPath != "/v1/nodes/node-1/stage/stage-abc/diff" {
		t.Errorf("unexpected request path: got %q", gotPath)
	}

	// Top-level step_index should be present and match the stepIndex argument.
	rawStepIndex, ok := gotPayload["step_index"]
	if !ok {
		t.Fatalf("payload missing top-level step_index: %#v", gotPayload)
	}
	if v, ok := rawStepIndex.(float64); !ok || int(v) != stepIndex {
		t.Errorf("top-level step_index=%v, want %d", rawStepIndex, stepIndex)
	}

	// Summary should be a JSON object containing step_index as well.
	rawSummary, ok := gotPayload["summary"]
	if !ok {
		t.Fatalf("payload missing summary: %#v", gotPayload)
	}

	summary, ok := rawSummary.(map[string]any)
	if !ok {
		t.Fatalf("summary has unexpected type %T", rawSummary)
	}

	rawSummaryStepIndex, ok := summary["step_index"]
	if !ok {
		t.Fatalf("summary missing step_index: %#v", summary)
	}
	if v, ok := rawSummaryStepIndex.(float64); !ok || int(v) != stepIndex {
		t.Errorf("summary step_index=%v, want %d", rawSummaryStepIndex, stepIndex)
	}
}
