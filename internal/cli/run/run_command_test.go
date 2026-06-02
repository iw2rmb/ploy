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

func TestRunCommandSubmitRemoteSelector(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine\n"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repo_url":   "https://gitlab.example.com/team/repo.git",
				"ref":        "master",
				"ref_is_sha": false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"run_id":  runID.String(),
				"mig_id":  migID.String(),
				"spec_id": specID.String(),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	var stdout, stderr bytes.Buffer
	if err := executeRunCommand([]string{specPath, "team/repo"}, &stdout, &stderr); err != nil {
		t.Fatalf("run submit: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("run_id: "+runID.String())) {
		t.Fatalf("expected run id in stdout, got %q", stdout.String())
	}
}

func TestRunCommandSBOMDiff(t *testing.T) {
	runID := domaintypes.NewRunID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/"+runID.String()+"/sbom/diff" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"run_id": runID.String(),
			"view":   "diff",
			"packages": []map[string]any{
				{"package": "alpha", "version_pre": "1.0", "version_post": "2.0", "change": "changed"},
			},
		})
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	var stdout, stderr bytes.Buffer
	if err := executeRunCommand([]string{"sbom", "diff", runID.String()}, &stdout, &stderr); err != nil {
		t.Fatalf("run sbom diff: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if want := "SBOM diff\nalpha                    1.0              -> 2.0\n"; stdout.String() != want {
		t.Fatalf("stdout=%q, want %q", stdout.String(), want)
	}
}

func TestRunCommandSBOMDisabledBuildGateError(t *testing.T) {
	runID := domaintypes.NewRunID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "build gate disabled for run", http.StatusBadRequest)
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	var stdout, stderr bytes.Buffer
	err := executeRunCommand([]string{"sbom", "diff", runID.String()}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "build gate disabled for run" {
		t.Fatalf("error=%q, want control-plane body", err.Error())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
}
