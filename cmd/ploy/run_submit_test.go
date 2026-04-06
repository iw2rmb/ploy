package main

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

// TestRunSubmitCallsControlPlane validates `ploy run --repo ... --base-ref ... --target-ref ... --spec ...`
// calls POST /v1/runs and prints run_id and mig_id.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestRunSubmitCallsControlPlane(t *testing.T) {
	t.Setenv("USER", "test-user")

	// Create a temporary spec file for the test.
	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "spec.yaml")
	specContent := `steps:
  - image: alpine:latest
    command: echo hello
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	var submitCalled bool
	var capturedRequest map[string]any

	runID := domaintypes.NewRunID().String()
	migID := domaintypes.NewMigID().String()
	specID := domaintypes.NewSpecID().String()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle POST /v1/runs for run submission.
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs" {
			submitCalled = true

			// Capture and validate the request body.
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Errorf("decode request body: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			// Return 201 Created with run_id.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run_id":"` + runID + `","mig_id":"` + migID + `","spec_id":"` + specID + `"}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{
		"run",
		"--repo", "https://github.com/test/repo",
		"--base-ref", "main",
		"--target-ref", "feature-branch",
		"--spec", specPath,
	}, &buf)
	if err != nil {
		t.Fatalf("run submit error: %v", err)
	}

	if !submitCalled {
		t.Fatal("expected POST /v1/runs to be called")
	}

	// Validate captured request fields.
	if capturedRequest["repo_url"] != "https://github.com/test/repo" {
		t.Errorf("expected repo_url='https://github.com/test/repo', got %v", capturedRequest["repo_url"])
	}
	if capturedRequest["base_ref"] != "main" {
		t.Errorf("expected base_ref='main', got %v", capturedRequest["base_ref"])
	}
	if capturedRequest["target_ref"] != "feature-branch" {
		t.Errorf("expected target_ref='feature-branch', got %v", capturedRequest["target_ref"])
	}
	if capturedRequest["created_by"] != "test-user" {
		t.Errorf("expected created_by='test-user', got %v", capturedRequest["created_by"])
	}
	spec, ok := capturedRequest["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec to be JSON object, got %T (%v)", capturedRequest["spec"], capturedRequest["spec"])
	}
	steps, ok := spec["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected spec.steps[0], got %T (%v)", spec["steps"], spec["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected spec.steps[0] to be object, got %T", steps[0])
	}
	if step0["image"] != "alpine:latest" {
		t.Errorf("expected spec.steps[0].image='alpine:latest', got %v", step0["image"])
	}
	if step0["command"] != "echo hello" {
		t.Errorf("expected spec.steps[0].command='echo hello', got %v", step0["command"])
	}

	// Validate output contains run_id and mig_id.
	output := buf.String()
	if !strings.Contains(output, "run_id: "+runID) {
		t.Errorf("expected output to contain run_id %q: %s", runID, output)
	}
	if !strings.Contains(output, "mig_id: "+migID) {
		t.Errorf("expected output to contain mig_id %q: %s", migID, output)
	}
}

// TestRunSubmitMissingFlags validates that missing required flags produce errors.
func TestRunSubmitMissingFlags(t *testing.T) {
	// Create a temporary spec file for some tests.
	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine\n"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing repo",
			args:    []string{"run", "--base-ref", "main", "--target-ref", "feature", "--spec", specPath},
			wantErr: "--repo is required",
		},
		{
			name:    "missing base-ref",
			args:    []string{"run", "--repo", "https://github.com/test/repo", "--target-ref", "feature", "--spec", specPath},
			wantErr: "--base-ref is required",
		},
		{
			name:    "missing target-ref",
			args:    []string{"run", "--repo", "https://github.com/test/repo", "--base-ref", "main", "--spec", specPath},
			wantErr: "--target-ref is required",
		},
		{
			name:    "missing spec",
			args:    []string{"run", "--repo", "https://github.com/test/repo", "--base-ref", "main", "--target-ref", "feature"},
			wantErr: "--spec is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := executeCmd(tc.args, &buf)
			if err == nil {
				t.Fatal("expected error for missing flag")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

// TestRunSubmitSpecFromStdin validates that --spec - reads from stdin.
func TestRunSubmitSpecFromStdin(t *testing.T) {
	t.Setenv("USER", "test-user")

	var submitCalled bool
	var capturedRequest map[string]any
	runID := domaintypes.NewRunID().String()
	migID := domaintypes.NewMigID().String()
	specID := domaintypes.NewSpecID().String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs" {
			submitCalled = true
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Errorf("decode request body: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run_id":"` + runID + `","mig_id":"` + migID + `","spec_id":"` + specID + `"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()

	_, _ = w.Write([]byte("steps:\n  - image: alpine:latest\n    command: echo stdin\n"))
	_ = w.Close()

	var buf bytes.Buffer
	err = executeCmd([]string{
		"run",
		"--repo", "https://github.com/test/repo",
		"--base-ref", "main",
		"--target-ref", "feature",
		"--spec", "-",
	}, &buf)
	if err != nil {
		t.Fatalf("run submit with stdin spec error: %v", err)
	}
	if !submitCalled {
		t.Fatal("expected POST /v1/runs to be called")
	}
	spec, ok := capturedRequest["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec to be JSON object, got %T (%v)", capturedRequest["spec"], capturedRequest["spec"])
	}
	steps, ok := spec["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected spec.steps[0], got %T (%v)", spec["steps"], spec["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected spec.steps[0] to be object, got %T", steps[0])
	}
	if step0["command"] != "echo stdin" {
		t.Errorf("expected spec.steps[0].command='echo stdin', got %v", step0["command"])
	}
}

// TestRunSubmitJSONSpec validates that JSON specs are accepted.
func TestRunSubmitJSONSpec(t *testing.T) {
	t.Setenv("USER", "test-user")

	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "spec.json")
	specContent := `{"steps":[{"image":"alpine:latest","command":"echo hello"}]}`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	var submitCalled bool
	runID := domaintypes.NewRunID().String()
	migID := domaintypes.NewMigID().String()
	specID := domaintypes.NewSpecID().String()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs" {
			submitCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run_id":"` + runID + `","mig_id":"` + migID + `","spec_id":"` + specID + `"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{
		"run",
		"--repo", "https://github.com/test/repo",
		"--base-ref", "main",
		"--target-ref", "feature",
		"--spec", specPath,
	}, &buf)
	if err != nil {
		t.Fatalf("run submit with JSON spec error: %v", err)
	}

	if !submitCalled {
		t.Fatal("expected POST /v1/runs to be called")
	}
	output := buf.String()
	if !strings.Contains(output, "run_id: "+runID) {
		t.Errorf("expected output to contain run_id %q: %s", runID, output)
	}
	if !strings.Contains(output, "mig_id: "+migID) {
		t.Errorf("expected output to contain mig_id %q: %s", migID, output)
	}
}

// TestRunSubmitInvalidSpec validates that invalid specs produce errors.
func TestRunSubmitInvalidSpec(t *testing.T) {
	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "invalid.yaml")

	// Write invalid YAML that is also not valid JSON.
	if err := os.WriteFile(specPath, []byte(`{invalid yaml`), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	var buf bytes.Buffer
	err := executeCmd([]string{
		"run",
		"--repo", "https://github.com/test/repo",
		"--base-ref", "main",
		"--target-ref", "feature",
		"--spec", specPath,
	}, &buf)

	if err == nil {
		t.Fatal("expected error for invalid spec")
	}
	if !strings.Contains(err.Error(), "load spec") && !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected error about spec parsing, got %q", err.Error())
	}
}

// TestRunSubmitNonExistentSpec validates that non-existent spec files produce errors.
func TestRunSubmitNonExistentSpec(t *testing.T) {
	var buf bytes.Buffer
	err := executeCmd([]string{
		"run",
		"--repo", "https://github.com/test/repo",
		"--base-ref", "main",
		"--target-ref", "feature",
		"--spec", "/nonexistent/path/spec.yaml",
	}, &buf)

	if err == nil {
		t.Fatal("expected error for non-existent spec file")
	}
	if !strings.Contains(err.Error(), "load spec") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("expected error about missing file, got %q", err.Error())
	}
}

func TestRunSubmitAcceptsMigratedSingleRepoFlags(t *testing.T) {
	t.Setenv("USER", "test-user")

	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "spec.yaml")
	specContent := `steps:
  - image: alpine:3.20
    command: echo from-spec
envs:
  KEY1: from-spec
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	var capturedRequest map[string]any
	runID := domaintypes.NewRunID().String()
	migID := domaintypes.NewMigID().String()
	specID := domaintypes.NewSpecID().String()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs" {
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run_id":"` + runID + `","mig_id":"` + migID + `","spec_id":"` + specID + `"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{
		"run",
		"--repo", "https://github.com/acme/service.git",
		"--base-ref", "main",
		"--target-ref", "ploy/java17",
		"--spec", specPath,
		"--job-env", "KEY1=from-cli",
		"--job-env", "KEY2=extra",
		"--job-image", "ghcr.io/acme/mig:2",
		"--job-command", `["echo","from-cli"]`,
		"--gitlab-pat", "glpat-test123",
		"--gitlab-domain", "gitlab.example.com",
		"--mr-success",
		"--mr-fail",
	}, &buf)
	if err != nil {
		t.Fatalf("run submit error: %v", err)
	}

	spec, ok := capturedRequest["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec to be JSON object, got %T (%v)", capturedRequest["spec"], capturedRequest["spec"])
	}
	steps, ok := spec["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected spec.steps[0], got %T (%v)", spec["steps"], spec["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected spec.steps[0] to be object, got %T", steps[0])
	}
	if got := step0["image"]; got != "ghcr.io/acme/mig:2" {
		t.Fatalf("expected steps[0].image override, got %v", got)
	}
	command, ok := step0["command"].([]any)
	if !ok || len(command) != 2 || command[0] != "echo" || command[1] != "from-cli" {
		t.Fatalf("expected steps[0].command JSON array override, got %v", step0["command"])
	}
	envs, ok := spec["envs"].(map[string]any)
	if !ok {
		t.Fatalf("expected envs in spec, got %T", spec["envs"])
	}
	if envs["KEY1"] != "from-cli" || envs["KEY2"] != "extra" {
		t.Fatalf("expected envs overrides, got %v", envs)
	}
	if spec["gitlab_pat"] != "glpat-test123" {
		t.Fatalf("expected gitlab_pat in spec, got %v", spec["gitlab_pat"])
	}
	if spec["gitlab_domain"] != "gitlab.example.com" {
		t.Fatalf("expected gitlab_domain in spec, got %v", spec["gitlab_domain"])
	}
	if spec["mr_on_success"] != true || spec["mr_on_fail"] != true {
		t.Fatalf("expected mr flags in spec, got success=%v fail=%v", spec["mr_on_success"], spec["mr_on_fail"])
	}

	// Ensure PAT is not echoed back in textual output.
	out := buf.String()
	if strings.Contains(out, "glpat-test123") {
		t.Fatalf("PAT should not appear in output: %s", out)
	}
}

func TestRunSubmitFollowUsesRunStatusFormat(t *testing.T) {
	t.Setenv("USER", "test-user")

	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "spec.yaml")
	specContent := `steps:
  - image: alpine:latest
    command: echo hello
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()
	repoCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":  runID.String(),
				"mig_id":  migID.String(),
				"spec_id": specID.String(),
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String():
			status := "running"
			if repoCalls >= 2 {
				status = "success"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         runID.String(),
				"status":     status,
				"mig_id":     migID.String(),
				"mig_name":   "java17-upgrade",
				"spec_id":    specID.String(),
				"created_at": "2026-02-24T08:00:00Z",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/status":
			stageState := "running"
			if repoCalls >= 2 {
				stageState = "succeeded"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id": runID.String(),
				"state":  stageState,
				"stages": map[string]any{
					jobID.String(): map[string]any{
						"state":        stageState,
						"attempts":     1,
						"max_attempts": 1,
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos":
			repoCalls++
			repoStatus := "Running"
			if repoCalls >= 2 {
				repoStatus = "Success"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repos": []map[string]any{
					{
						"run_id":     runID.String(),
						"repo_id":    repoID.String(),
						"repo_url":   "https://github.com/acme/service.git",
						"base_ref":   "main",
						"target_ref": "ploy/java17",
						"status":     repoStatus,
						"attempt":    1,
						"created_at": "2026-02-24T08:01:00Z",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs":
			jobStatus := "Running"
			duration := int64(0)
			if repoCalls >= 2 {
				jobStatus = "Success"
				duration = 1200
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":  runID.String(),
				"repo_id": repoID.String(),
				"attempt": 1,
				"jobs": []map[string]any{
					{
						"job_id":      jobID.String(),
						"name":        "mig-0",
						"job_type":    "mig",
						"job_image":   "ghcr.io/acme/mig:1",
						"status":      jobStatus,
						"duration_ms": duration,
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs":
			_ = json.NewEncoder(w).Encode(map[string]any{"diffs": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{
		"run",
		"--repo", "https://github.com/acme/service.git",
		"--base-ref", "main",
		"--target-ref", "ploy/java17",
		"--spec", specPath,
		"--follow",
	}, &buf)
	if err != nil {
		t.Fatalf("run submit --follow error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "   Mig:   "+migID.String()+"   | java17-upgrade") {
		t.Fatalf("expected run status mig header, got: %q", out)
	}
	if !strings.Contains(out, "   Spec:  "+specID.String()+" ("+server.URL+"/v1/migs/"+migID.String()+"/specs/latest)") {
		t.Fatalf("expected run status spec download link, got: %q", out)
	}
	if !strings.Contains(out, "   [1/1] github.com/acme/service (https://github.com/acme/service.git) main -> ploy/java17") {
		t.Fatalf("expected run status repo header, got: %q", out)
	}
	if strings.Count(out, "   Mig:   "+migID.String()+"   | java17-upgrade") != 1 {
		t.Fatalf("expected mig header to render once in follow output, got: %q", out)
	}
	if strings.Count(out, "   [1/1] github.com/acme/service (https://github.com/acme/service.git) main -> ploy/java17") != 1 {
		t.Fatalf("expected repo header to render once in follow output, got: %q", out)
	}
	if strings.Contains(out, "run_id: ") || strings.Contains(out, "mig_id: ") {
		t.Fatalf("expected follow mode to omit preface id lines, got: %q", out)
	}
	if strings.Contains(out, "Repo 1/1:") {
		t.Fatalf("expected old follow format to be absent, got: %q", out)
	}
	if !strings.Contains(out, "Final state: succeeded") {
		t.Fatalf("expected final success line, got: %q", out)
	}
}
