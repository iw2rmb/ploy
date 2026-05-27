package run

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func executeRunCommand(args []string, stdout, stderr *bytes.Buffer) error {
	cmd := NewCommand()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd.Execute()
}

func TestRunCommandSubmitJSONWritesStdoutOnly(t *testing.T) {
	t.Setenv("USER", "test-user")

	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine\n"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"run_id":  runID.String(),
				"mig_id":  migID.String(),
				"spec_id": specID.String(),
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	var stdout, stderr bytes.Buffer
	if err := executeRunCommand([]string{
		"--repo", "https://github.com/test/repo",
		"--base-ref", "main",
		"--target-ref", "feature",
		"--spec", specPath,
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("run --json: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	var parsed map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("decode stdout JSON %q: %v", stdout.String(), err)
	}
	if got := parsed["run_id"]; got != runID.String() {
		t.Fatalf("run_id = %v, want %s", got, runID.String())
	}
	if _, ok := parsed["mig_id"]; ok {
		t.Fatalf("did not expect legacy text fields in JSON output: %s", stdout.String())
	}
}
