package run

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestRunCommand_RepoConflictReportsCurrentRunID(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runID := domaintypes.NewRunID()
	message := "repository https://gitlab.example.com/team/repo already has an active migration; current Run ID: " + runID.String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/repos/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repo_url": "https://gitlab.example.com/team/repo.git", "ref": "feature/other", "ref_is_sha": false,
			})
		case "/v1/runs":
			http.Error(w, message, http.StatusConflict)
		default:
			t.Errorf("unexpected request after conflict: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

	var stdout, stderr bytes.Buffer
	err := executeRunCommand([]string{specPath, "team/repo:feature/other"}, &stdout, &stderr)
	// Submission fails and displays the existing Run ID instead of creating a run.
	if err == nil || !strings.Contains(err.Error(), message) {
		t.Fatalf("CLI error=%v, want %q", err, message)
	}
	if strings.Contains(stdout.String(), "run_id:") {
		t.Fatalf("conflict printed successful submission: %q", stdout.String())
	}
}

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
	clienv.UseControlPlaneEnv(t, server.URL)

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
	clienv.UseControlPlaneEnv(t, server.URL)

	{
		args := []string{"sbom", "diff", runID.String()}
		want := "SBOM diff\nalpha 1.0              -> 2.0\n"
		var stdout, stderr bytes.Buffer
		if err := executeRunCommand(args, &stdout, &stderr); err != nil {
			t.Fatal(err)
		}
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("stdout=%q stderr=%q, want %q", stdout.String(), stderr.String(), want)
		}
	}
}

func TestRunCommandSBOMDisabledBuildGateError(t *testing.T) {
	runID := domaintypes.NewRunID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "build gate disabled for run", http.StatusBadRequest)
	}))
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

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

// All three entry points render the same available gate outcomes.
func TestRunCommandsGateOutcomeLinks(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runID := domaintypes.NewRunID()
	preID := domaintypes.NewJobID()
	postID := domaintypes.NewJobID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		switch r.URL.Path {
		case "/v1/repos/resolve":
			body = map[string]any{"repo_url": "https://gitlab.example.com/team/repo.git", "ref": "main", "ref_is_sha": false}
		case "/v1/runs":
			w.WriteHeader(http.StatusCreated)
			body = map[string]any{"run_id": runID, "mig_id": domaintypes.NewMigID(), "spec_id": domaintypes.NewSpecID()}
		case "/v1/runs/" + runID.String():
			body = map[string]any{"id": runID, "status": "Success", "attempt": 1}
		case "/v1/runs/" + runID.String() + "/status":
			body = map[string]any{"run_id": runID, "state": "succeeded", "stages": map[string]any{
				preID.String():  map[string]any{"state": "succeeded", "artifacts": map[string]string{"sbom": "pre-sbom", "cves": "pre-cves"}},
				postID.String(): map[string]any{"state": "succeeded", "artifacts": map[string]string{"sbom": "post-sbom", "cves": "post-cves"}},
			}}
		case "/v1/runs/" + runID.String() + "/jobs":
			body = map[string]any{"jobs": []map[string]any{
				{"job_id": preID, "job_type": "pre_gate", "status": "Success", "next_id": postID},
				{"job_id": postID, "job_type": "post_gate", "status": "Success"},
			}}
		case "/v1/runs/" + runID.String() + "/diffs":
			body = map[string]any{"diffs": []any{}}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)
	t.Setenv("PLOY_AUTH_TOKEN", "fixture+token")
	for _, args := range [][]string{{specPath, "team/repo", "--follow"}, {"status", runID.String()}, {"status", runID.String(), "--follow"}} {
		var out, errOut bytes.Buffer
		if err := executeRunCommand(args, &out, &errOut); err != nil {
			t.Fatal(err)
		}
		for _, link := range []string{
			"SBOM (" + server.URL + "/v1/jobs/" + preID.String() + "/sbom?auth_token=fixture%2Btoken)",
			"DIFF (" + server.URL + "/v1/jobs/" + postID.String() + "/sbom-diff?auth_token=fixture%2Btoken)",
			"CVEs (" + server.URL + "/v1/jobs/" + postID.String() + "/cves?auth_token=fixture%2Btoken)",
			server.URL + "/v1/jobs/" + preID.String() + "/logs?auth_token=fixture%2Btoken",
		} {
			if !strings.Contains(out.String(), link) {
				t.Fatalf("args=%v: missing %s in %q", args, link, out.String())
			}
		}
	}
}
