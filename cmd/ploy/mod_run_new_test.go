package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
			RunID  string `json:"run_id"`
			Status string `json:"status"`
		}{RunID: "mods-server-123", Status: "pending"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	args := []string{"--repo-url", "https://example.com/repo.git", "--repo-target-ref", "feature"}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}
	if received.Repository != "https://example.com/repo.git" {
		t.Fatalf("unexpected repository: %s", received.Repository)
	}
	if received.Metadata["repo_target_ref"] != "feature" {
		t.Fatalf("expected repo target metadata, got %v", received.Metadata)
	}
	if len(received.Stages) != 5 {
		t.Fatalf("expected 5 stages, got %d", len(received.Stages))
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
			RunID  string `json:"run_id"`
			Status string `json:"status"`
		}{RunID: "mods-abc123", Status: "pending"})
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	if err := executeModRun([]string{}, io.Discard); err != nil {
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
			RunID  string `json:"run_id"`
			Status string `json:"status"`
		}{RunID: "mods-gitlab-test", Status: "pending"})
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	args := []string{
		"--repo-url", "https://example.com/repo.git",
		"--repo-base-ref", "main",
		"--repo-target-ref", "feature",
		"--gitlab-pat", "glpat-test123",
		"--gitlab-domain", "gitlab.example.com",
		"--mr-success",
		"--mr-fail",
	}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}

	// Verify repository fields are set correctly.
	if received.Repository != "https://example.com/repo.git" {
		t.Fatalf("expected repo_url https://example.com/repo.git, got %s", received.Repository)
	}
	if received.Metadata["repo_base_ref"] != "main" {
		t.Fatalf("expected base_ref main, got %s", received.Metadata["repo_base_ref"])
	}
	if received.Metadata["repo_target_ref"] != "feature" {
		t.Fatalf("expected target_ref feature, got %s", received.Metadata["repo_target_ref"])
	}

	// Verify PAT is not printed in output.
	output := buf.String()
	if strings.Contains(output, "glpat-test123") {
		t.Fatalf("PAT should not appear in output: %s", output)
	}
}
