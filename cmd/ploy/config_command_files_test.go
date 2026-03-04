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

	"github.com/iw2rmb/ploy/internal/testutil/clienv"
	"github.com/iw2rmb/ploy/internal/testutil/stdcapture"
)

// TestHandleConfigGitLabShowSuccess verifies the 'show' subcommand retrieves and displays
// GitLab configuration from the server, redacting sensitive token information.
func TestHandleConfigGitLabShowSuccess(t *testing.T) {
	// Arrange a fake config endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/config/gitlab" {
			t.Fatalf("expected /v1/config/gitlab, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"domain": "https://gitlab.example.com",
			"token":  "glpat-abcdef123456",
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigGitLabShow(nil, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigGitLabShow error: %v", err)
	}

	if !strings.Contains(output, "Domain: https://gitlab.example.com") {
		t.Fatalf("expected domain in output, got: %q", output)
	}
	// Token should be redacted.
	if !strings.Contains(output, "Token:") {
		t.Fatalf("expected token label in output, got: %q", output)
	}
	if strings.Contains(output, "glpat-abcdef123456") {
		t.Fatalf("token should be redacted, got: %q", output)
	}
}

// TestHandleConfigGitLabShowRedactsShortToken ensures that even short tokens
// are fully redacted in the output.
func TestHandleConfigGitLabShowRedactsShortToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/config/gitlab" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"domain": "https://gitlab.example.com",
			"token":  "short",
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	out := stdcapture.CaptureStdout(t, func() {
		err = handleConfigGitLabShow(nil, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigGitLabShow error: %v", err)
	}
	if strings.Contains(out, "short") {
		t.Fatalf("token must be redacted for short tokens, got: %q", out)
	}
	if !strings.Contains(out, "Token:  ***") {
		t.Fatalf("expected redaction marker, got: %q", out)
	}
}

// TestHandleConfigGitLabSetSuccess verifies that the 'set' subcommand reads a JSON
// file and sends a PUT request to the server with the correct payload.
func TestHandleConfigGitLabSetSuccess(t *testing.T) {
	// Prepare a temporary config file.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "gitlab.json")
	configData := map[string]any{
		"domain": "https://gitlab.example.com",
		"token":  "glpat-test123",
	}
	configJSON, _ := json.Marshal(configData)
	if err := os.WriteFile(configPath, configJSON, 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	// Arrange a fake PUT endpoint.
	var gotPath, gotMethod, gotContentType string
	var gotBody gitLabConfigPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigGitLabSet([]string{"--file", configPath}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigGitLabSet error: %v", err)
	}

	// Assert request details.
	if gotMethod != "PUT" {
		t.Fatalf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/v1/config/gitlab" {
		t.Fatalf("expected /v1/config/gitlab, got %s", gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected application/json, got %s", gotContentType)
	}
	if gotBody.Domain != "https://gitlab.example.com" || gotBody.Token != "glpat-test123" {
		t.Fatalf("unexpected body: %+v", gotBody)
	}

	if !strings.Contains(output, "GitLab configuration updated successfully") {
		t.Fatalf("expected success message, got: %q", output)
	}
}

// TestHandleConfigGitLabSetInvalidJSON ensures that invalid JSON files are
// detected and rejected before making any server requests.
func TestHandleConfigGitLabSetInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(configPath, []byte("{invalid json}"), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	buf := &bytes.Buffer{}
	err := handleConfigGitLabSet([]string{"--file", configPath}, buf)
	if err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse JSON") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigGitLabSetValidationFailure verifies that files with missing
// required fields fail validation before being sent to the server.
func TestHandleConfigGitLabSetValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "incomplete.json")
	// Missing required token field.
	configData := map[string]any{
		"domain": "https://gitlab.example.com",
	}
	configJSON, _ := json.Marshal(configData)
	if err := os.WriteFile(configPath, configJSON, 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	buf := &bytes.Buffer{}
	err := handleConfigGitLabSet([]string{"--file", configPath}, buf)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "token is required") {
		t.Fatalf("expected token required error, got: %v", err)
	}
}

// TestHandleConfigGitLabValidateSuccess verifies that valid configuration files
// pass validation and produce a success message.
func TestHandleConfigGitLabValidateSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "valid.json")
	configData := map[string]any{
		"domain": "https://gitlab.example.com",
		"token":  "glpat-test123",
	}
	configJSON, _ := json.Marshal(configData)
	if err := os.WriteFile(configPath, configJSON, 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigGitLabValidate([]string{"--file", configPath}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigGitLabValidate error: %v", err)
	}

	if !strings.Contains(output, "GitLab configuration is valid") {
		t.Fatalf("expected validation success message, got: %q", output)
	}
}

// TestHandleConfigGitLabValidateInvalidJSON ensures that the validate command
// rejects files with invalid JSON syntax.
func TestHandleConfigGitLabValidateInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(configPath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	buf := &bytes.Buffer{}
	err := handleConfigGitLabValidate([]string{"--file", configPath}, buf)
	if err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse JSON") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigGitLabValidateFailure tests various validation failure scenarios
// where required fields are missing or empty in the configuration file.
func TestHandleConfigGitLabValidateFailure(t *testing.T) {
	tmpDir := t.TempDir()
	tests := []struct {
		name        string
		config      map[string]any
		expectedErr string
	}{
		{
			name:        "missing domain",
			config:      map[string]any{"token": "glpat-test"},
			expectedErr: "domain is required",
		},
		{
			name:        "missing token",
			config:      map[string]any{"domain": "https://gitlab.example.com"},
			expectedErr: "token is required",
		},
		{
			name:        "empty domain",
			config:      map[string]any{"domain": "", "token": "glpat-test"},
			expectedErr: "domain is required",
		},
		{
			name:        "empty token",
			config:      map[string]any{"domain": "https://gitlab.example.com", "token": ""},
			expectedErr: "token is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(tmpDir, tt.name+".json")
			configJSON, _ := json.Marshal(tt.config)
			if err := os.WriteFile(configPath, configJSON, 0o600); err != nil {
				t.Fatalf("write test config: %v", err)
			}

			buf := &bytes.Buffer{}
			err := handleConfigGitLabValidate([]string{"--file", configPath}, buf)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), "validation failed") {
				t.Fatalf("expected validation failed error, got: %v", err)
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Fatalf("expected %q in error, got: %v", tt.expectedErr, err)
			}
		})
	}
}

// TestHandleConfigGitLabSetServerError verifies that server-side errors are
// properly propagated to the user when setting configuration.
func TestHandleConfigGitLabSetServerError(t *testing.T) {
	// Valid config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "gitlab.json")
	configJSON := []byte(`{"domain":"https://gitlab.example.com","token":"glpat-err"}`)
	if err := os.WriteFile(configPath, configJSON, 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	// Server responds with error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()
	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	err := handleConfigGitLabSet([]string{"--file", configPath}, buf)
	if err == nil || !strings.Contains(err.Error(), "server returned 400: bad") {
		t.Fatalf("expected server error, got: %v", err)
	}
}
