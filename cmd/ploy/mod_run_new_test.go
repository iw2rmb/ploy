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

func TestExecuteModRunSubmitsTicket(t *testing.T) {
	var received modsapi.TicketSubmitRequest
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
		resp := modsapi.TicketSubmitResponse{Ticket: modsapi.TicketSummary{TicketID: received.TicketID, State: modsapi.TicketStatePending}}
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	t.Setenv(controlPlaneURLEnv, server.URL)

	buf := &bytes.Buffer{}
	args := []string{"--ticket", "mods-test", "--repo-url", "https://example.com/repo.git", "--repo-target-ref", "feature"}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}

	if received.TicketID != "mods-test" {
		t.Fatalf("expected ticket id mods-test, got %s", received.TicketID)
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
	if !strings.Contains(output, "Mods ticket mods-test submitted") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestExecuteModRunGeneratesTicket(t *testing.T) {
	var received modsapi.TicketSubmitRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		resp := modsapi.TicketSubmitResponse{Ticket: modsapi.TicketSummary{TicketID: received.TicketID, State: modsapi.TicketStatePending}}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv(controlPlaneURLEnv, server.URL)

	if err := executeModRun([]string{}, io.Discard); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}
	if received.TicketID == "" {
		t.Fatalf("expected generated ticket id")
	}
	if !strings.HasPrefix(received.TicketID, "mods-") {
		t.Fatalf("expected mods- prefix, got %s", received.TicketID)
	}
}

func TestExecuteModRunGitLabFlags(t *testing.T) {
	var receivedSpec map[string]any
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/mods" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		// Decode the request body to check for spec
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		attemptCount++
		// First attempt is canonical request, return 400 to trigger simplified retry
		if attemptCount == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "use simplified format"})
			return
		}
		// Second attempt should have spec field
		if specRaw, ok := payload["spec"]; ok {
			specBytes, _ := json.Marshal(specRaw)
			if err := json.Unmarshal(specBytes, &receivedSpec); err != nil {
				t.Fatalf("decode spec: %v", err)
			}
		}
		// Return 201 simplified response
		resp := map[string]string{
			"ticket_id":  "mods-gitlab-test",
			"status":     "pending",
			"repo_url":   "https://example.com/repo.git",
			"base_ref":   "main",
			"target_ref": "feature",
		}
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	t.Setenv(controlPlaneURLEnv, server.URL)

	buf := &bytes.Buffer{}
	args := []string{
		"--ticket", "mods-gitlab-test",
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

	// Verify Spec payload contains GitLab options
	if len(receivedSpec) == 0 {
		t.Fatalf("expected spec payload with GitLab options")
	}

	// Verify GitLab PAT is included (but redacted in logs)
	if pat, ok := receivedSpec["gitlab_pat"].(string); !ok || pat != "glpat-test123" {
		t.Fatalf("expected gitlab_pat in spec, got %v", receivedSpec["gitlab_pat"])
	}
	if domain, ok := receivedSpec["gitlab_domain"].(string); !ok || domain != "gitlab.example.com" {
		t.Fatalf("expected gitlab_domain in spec, got %v", receivedSpec["gitlab_domain"])
	}
	if success, ok := receivedSpec["mr_on_success"].(bool); !ok || !success {
		t.Fatalf("expected mr_on_success=true in spec, got %v", receivedSpec["mr_on_success"])
	}
	if fail, ok := receivedSpec["mr_on_fail"].(bool); !ok || !fail {
		t.Fatalf("expected mr_on_fail=true in spec, got %v", receivedSpec["mr_on_fail"])
	}

	// Verify PAT is not printed in output
	output := buf.String()
	if strings.Contains(output, "glpat-test123") {
		t.Fatalf("PAT should not appear in output: %s", output)
	}
}
