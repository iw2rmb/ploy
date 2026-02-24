package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

func TestExecuteModRunSubmitsRun(t *testing.T) {
	var received modsapi.RunSubmitRequest
	runID := domaintypes.NewRunID().String()
	modID := domaintypes.NewMigID().String()
	specID := domaintypes.NewSpecID().String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			// Server returns 201 Created with {run_id, mod_id, spec_id}.
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(struct {
				RunID  string `json:"run_id"`
				MigID  string `json:"mig_id"`
				SpecID string `json:"spec_id"`
			}{RunID: runID, MigID: modID, SpecID: specID}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID+"/status":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStatePending,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: docker.io/test/image:v1\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	buf := &bytes.Buffer{}
	args := []string{
		"--repo-url", "https://example.com/repo.git",
		"--repo-base-ref", "main",
		"--repo-target-ref", "feature",
		"--spec", specPath,
	}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}
	if received.RepoURL.String() != "https://example.com/repo.git" {
		t.Fatalf("unexpected repo_url: %s", received.RepoURL.String())
	}
	if received.BaseRef.String() != "main" {
		t.Fatalf("expected base_ref main, got %s", received.BaseRef.String())
	}
	if received.TargetRef.String() != "feature" {
		t.Fatalf("expected target_ref feature, got %s", received.TargetRef.String())
	}
	if len(received.Spec) == 0 {
		t.Fatalf("expected non-empty spec payload")
	}
	output := buf.String()
	if !strings.Contains(output, "  Run: "+runID+" submitted") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestExecuteModRunServerAssignsRunID(t *testing.T) {
	var received modsapi.RunSubmitRequest
	runID := domaintypes.NewRunID().String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			_ = json.NewDecoder(r.Body).Decode(&received)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(struct {
				RunID  string `json:"run_id"`
				MigID  string `json:"mig_id"`
				SpecID string `json:"spec_id"`
			}{RunID: runID, MigID: domaintypes.NewMigID().String(), SpecID: domaintypes.NewSpecID().String()})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID+"/status":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStatePending,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	if err := executeModRun([]string{
		"--repo-url", "https://example.com/repo.git",
		"--repo-base-ref", "main",
		"--repo-target-ref", "feature",
	}, io.Discard); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}
}

func TestExecuteModRunGitLabFlags(t *testing.T) {
	var received modsapi.RunSubmitRequest
	runID := domaintypes.NewRunID().String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(struct {
				RunID  string `json:"run_id"`
				MigID  string `json:"mig_id"`
				SpecID string `json:"spec_id"`
			}{RunID: runID, MigID: domaintypes.NewMigID().String(), SpecID: domaintypes.NewSpecID().String()})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID+"/status":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStatePending,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: docker.io/test/image:v1\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	buf := &bytes.Buffer{}
	args := []string{
		"--repo-url", "https://example.com/repo.git",
		"--repo-base-ref", "main",
		"--repo-target-ref", "feature",
		"--spec", specPath,
		"--gitlab-pat", "glpat-test123",
		"--gitlab-domain", "gitlab.example.com",
		"--mr-success",
		"--mr-fail",
	}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}

	// Verify repository fields are set correctly.
	if received.RepoURL.String() != "https://example.com/repo.git" {
		t.Fatalf("expected repo_url https://example.com/repo.git, got %s", received.RepoURL.String())
	}
	if received.BaseRef.String() != "main" {
		t.Fatalf("expected base_ref main, got %s", received.BaseRef.String())
	}
	if received.TargetRef.String() != "feature" {
		t.Fatalf("expected target_ref feature, got %s", received.TargetRef.String())
	}

	// Verify PAT is not printed in output.
	output := buf.String()
	if strings.Contains(output, "glpat-test123") {
		t.Fatalf("PAT should not appear in output: %s", output)
	}
}
