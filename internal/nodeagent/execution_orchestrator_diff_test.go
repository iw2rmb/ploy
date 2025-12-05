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

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// stubDiffGen is a minimal DiffGenerator used for testing uploadDiffForStep.
type stubDiffGen struct{}

func (stubDiffGen) Generate(_ context.Context, _ string) ([]byte, error) {
	return []byte("diff-bytes"), nil
}

func (stubDiffGen) GenerateBetween(_ context.Context, _, _ string) ([]byte, error) {
	return []byte("diff-between-bytes"), nil
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
		defer func() {
			_ = r.Body.Close()
		}()

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
		jobs: make(map[string]*jobContext),
	}

	ctx := context.Background()
	stepIndex := types.StepIndex(2000)

	rc.uploadDiffForStep(
		ctx,
		"run-123",
		"stage-abc",
		"mod-0", // Job name (mainline mod job).
		stubDiffGen{},
		"/unused/workspace",
		step.Result{},
		stepIndex,
	)

	if gotPayload == nil {
		t.Fatalf("no payload captured from diff upload")
	}

	// Verify request path uses job-scoped endpoint.
	if gotPath != "/v1/runs/run-123/jobs/stage-abc/diff" {
		t.Errorf("unexpected request path: got %q, want /v1/runs/run-123/jobs/stage-abc/diff", gotPath)
	}

	// step_index is no longer at top level (it's derived from job's step_index in DB).
	// Summary should contain step_index for metadata purposes.
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
	if v, ok := rawSummaryStepIndex.(float64); !ok || types.StepIndex(v) != stepIndex {
		t.Errorf("summary step_index=%v, want %.0f", rawSummaryStepIndex, stepIndex)
	}
}
