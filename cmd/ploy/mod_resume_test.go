package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestModResumeCallsControlPlane verifies the CLI wires through to POST /v1/mods/{id}/resume
// and handles a successful 202 Accepted response.
func TestModResumeCallsControlPlane(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/mods/run-9/resume" {
			called = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"state":"running"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "resume", "run-9"}, buf)
	if err != nil {
		t.Fatalf("mod resume error: %v", err)
	}
	if !called {
		t.Fatalf("expected /v1/mods/{id}/resume to be called")
	}
	// Verify success message is printed.
	if !strings.Contains(buf.String(), "Resume requested") {
		t.Errorf("expected output to contain 'Resume requested', got: %s", buf.String())
	}
}

// TestModResumeIdempotent verifies the CLI handles 200 OK (idempotent resume) correctly.
func TestModResumeIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/mods/running-run/resume" {
			// 200 OK indicates run is already running (idempotent).
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "resume", "running-run"}, buf)
	if err != nil {
		t.Fatalf("mod resume should succeed on 200 OK: %v", err)
	}
}

// TestModResumeNotFound verifies the CLI returns an error when the run does not exist.
func TestModResumeNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/mods/nonexistent/resume" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("run not found"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "resume", "nonexistent"}, buf)
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to mention 'not found', got: %v", err)
	}
}

// TestModResumeConflict verifies the CLI returns an error when the run cannot be resumed
// (e.g., run already succeeded).
func TestModResumeConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/mods/succeeded-run/resume" {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte("cannot resume succeeded run"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "resume", "succeeded-run"}, buf)
	if err == nil {
		t.Fatal("expected error for non-resumable run")
	}
	if !strings.Contains(err.Error(), "cannot resume") {
		t.Errorf("expected error to mention 'cannot resume', got: %v", err)
	}
}

// TestModResumeBadRequest verifies the CLI returns an error for invalid run ID format.
// Note: IDs are now KSUID strings (not UUIDs); the server returns a generic "invalid id" message.
func TestModResumeBadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/mods/") {
			w.WriteHeader(http.StatusBadRequest)
			// Server returns "invalid id" for malformed run IDs (KSUID strings).
			_, _ = w.Write([]byte("invalid id: malformed run identifier"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	// Use a clearly invalid ID format for testing error handling.
	err := execute([]string{"mod", "resume", "bad-id"}, buf)
	if err == nil {
		t.Fatal("expected error for invalid run id")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected error to mention 'invalid', got: %v", err)
	}
}

// TestModResumeServerError verifies the CLI handles 5xx server errors gracefully.
func TestModResumeServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/mods/run-err/resume" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("database unavailable"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "resume", "run-err"}, buf)
	if err == nil {
		t.Fatal("expected error for server error")
	}
	if !strings.Contains(err.Error(), "database unavailable") {
		t.Errorf("expected error to contain server message, got: %v", err)
	}
}

// TestModResumeMissingRunID verifies the CLI validates that a run id argument is required.
func TestModResumeMissingRunID(t *testing.T) {
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "resume"}, buf)
	if err == nil {
		t.Fatal("expected error when run id is missing")
	}
	if !strings.Contains(err.Error(), "run id required") {
		t.Errorf("expected error to mention 'run id required', got: %v", err)
	}
}
