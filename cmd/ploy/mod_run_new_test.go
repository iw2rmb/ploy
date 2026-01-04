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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/mods" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		// Server returns 201 Created with canonical submit response.
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(struct {
			RunID  domaintypes.RunID `json:"run_id"`
			Status string            `json:"status"`
		}{RunID: domaintypes.RunID("mods-server-123"), Status: "pending"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	if err := os.WriteFile(specPath, []byte("image: docker.io/test/image:v1\n"), 0o644); err != nil {
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
	if received.RepoURL != "https://example.com/repo.git" {
		t.Fatalf("unexpected repo_url: %s", received.RepoURL)
	}
	if received.BaseRef != "main" {
		t.Fatalf("expected base_ref main, got %s", received.BaseRef)
	}
	if received.TargetRef != "feature" {
		t.Fatalf("expected target_ref feature, got %s", received.TargetRef)
	}
	if len(received.Spec) == 0 {
		t.Fatalf("expected non-empty spec payload")
	}
	output := buf.String()
	if !strings.Contains(output, "Mods run mods-server-123 submitted") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestExecuteModRunServerAssignsRunID(t *testing.T) {
	var received modsapi.RunSubmitRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		// Server returns 201 Created with canonical submit response.
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			RunID  domaintypes.RunID `json:"run_id"`
			Status string            `json:"status"`
		}{RunID: domaintypes.RunID("mods-abc123"), Status: "pending"})
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	if err := executeModRun([]string{
		"--repo-url", "https://example.com/repo.git",
		"--repo-base-ref", "main",
	}, io.Discard); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}
}

func TestExecuteModRunGitLabFlags(t *testing.T) {
	var received modsapi.RunSubmitRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/mods" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		// Server returns 201 Created with canonical submit response.
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			RunID  domaintypes.RunID `json:"run_id"`
			Status string            `json:"status"`
		}{RunID: domaintypes.RunID("mods-gitlab-test"), Status: "pending"})
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	if err := os.WriteFile(specPath, []byte("image: docker.io/test/image:v1\n"), 0o644); err != nil {
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
	if received.RepoURL != "https://example.com/repo.git" {
		t.Fatalf("expected repo_url https://example.com/repo.git, got %s", received.RepoURL)
	}
	if received.BaseRef != "main" {
		t.Fatalf("expected base_ref main, got %s", received.BaseRef)
	}
	if received.TargetRef != "feature" {
		t.Fatalf("expected target_ref feature, got %s", received.TargetRef)
	}

	// Verify PAT is not printed in output.
	output := buf.String()
	if strings.Contains(output, "glpat-test123") {
		t.Fatalf("PAT should not appear in output: %s", output)
	}
}
