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
)

// TestRunSubmitCallsControlPlane validates `ploy run --repo ... --base-ref ... --target-ref ... --spec ...`
// calls POST /v1/runs and prints run_id and mod_id.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestRunSubmitCallsControlPlane(t *testing.T) {
	t.Setenv("USER", "test-user")

	// Create a temporary spec file for the test.
	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "spec.yaml")
	specContent := `image: alpine:latest
command: echo hello
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	var submitCalled bool
	var capturedRequest map[string]any

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
			_, _ = w.Write([]byte(`{"run_id":"run-test-123","mod_id":"mod-test-456","spec_id":"spec-test-789"}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

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
	if spec["image"] != "alpine:latest" {
		t.Errorf("expected spec.image='alpine:latest', got %v", spec["image"])
	}
	if spec["command"] != "echo hello" {
		t.Errorf("expected spec.command='echo hello', got %v", spec["command"])
	}

	// Validate output contains run_id and mod_id.
	output := buf.String()
	if !strings.Contains(output, "run_id: run-test-123") {
		t.Errorf("expected output to contain run_id 'run-test-123': %s", output)
	}
	if !strings.Contains(output, "mod_id: mod-test-456") {
		t.Errorf("expected output to contain mod_id 'mod-test-456': %s", output)
	}
}

// TestRunSubmitMissingFlags validates that missing required flags produce errors.
func TestRunSubmitMissingFlags(t *testing.T) {
	// Create a temporary spec file for some tests.
	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "spec.yaml")
	if err := os.WriteFile(specPath, []byte(`image: alpine`), 0o644); err != nil {
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
			_, _ = w.Write([]byte(`{"run_id":"run-stdin-test","mod_id":"mod-stdin-test","spec_id":"spec-stdin-test"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	useServerDescriptor(t, server.URL)

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

	_, _ = w.Write([]byte("image: alpine:latest\ncommand: echo stdin\n"))
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
	if spec["command"] != "echo stdin" {
		t.Errorf("expected spec.command='echo stdin', got %v", spec["command"])
	}
}

// TestRunSubmitJSONSpec validates that JSON specs are accepted.
func TestRunSubmitJSONSpec(t *testing.T) {
	t.Setenv("USER", "test-user")

	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "spec.json")
	specContent := `{"image":"alpine:latest","command":"echo hello"}`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	var submitCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs" {
			submitCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run_id":"run-json-test","mod_id":"mod-json-test","spec_id":"spec-json-test"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

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
	if !strings.Contains(output, "run_id: run-json-test") {
		t.Errorf("expected output to contain run_id 'run-json-test': %s", output)
	}
	if !strings.Contains(output, "mod_id: mod-json-test") {
		t.Errorf("expected output to contain mod_id 'mod-json-test': %s", output)
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
