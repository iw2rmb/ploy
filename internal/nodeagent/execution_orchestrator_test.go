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

// TestUploadDiffForStep_TagsStepIndex verifies that uploadDiffForStep includes
// step_index in the diff summary for proper ordering in multi-step runs.
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
