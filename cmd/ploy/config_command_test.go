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
)

func TestHandleConfigRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfig(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing config subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy config") {
		t.Fatalf("expected config usage output, got: %q", out)
	}
}

func TestHandleConfigUnknownSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfig([]string{"unknown"}, buf)
	if err == nil {
		t.Fatalf("expected error for unknown config subcommand")
	}
	if !strings.Contains(err.Error(), "unknown config subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleConfigGitLabRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLab(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing gitlab subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy config gitlab") {
		t.Fatalf("expected gitlab usage output, got: %q", out)
	}
}

func TestHandleConfigGitLabUnknownSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLab([]string{"unknown"}, buf)
	if err == nil {
		t.Fatalf("expected error for unknown gitlab subcommand")
	}
	if !strings.Contains(err.Error(), "unknown gitlab subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}

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

	// Set up environment to use the test server.
	t.Setenv("PLOY_CONTROL_PLANE_URL", srv.URL)

	buf := &bytes.Buffer{}
	// Redirect stdout to capture output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err := handleConfigGitLabShow(nil, buf)

	// Close write end and read captured output.
	w.Close()
	var capturedOutput bytes.Buffer
	_, _ = io.Copy(&capturedOutput, r)

	if err != nil {
		t.Fatalf("handleConfigGitLabShow error: %v", err)
	}

	output := capturedOutput.String()
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

func TestHandleConfigGitLabShowRedactsShortToken(t *testing.T) {
	// Arrange a fake config endpoint with a short token.
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

	t.Setenv("PLOY_CONTROL_PLANE_URL", srv.URL)

	buf := &bytes.Buffer{}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err := handleConfigGitLabShow(nil, buf)

	w.Close()
	var capturedOutput bytes.Buffer
	_, _ = io.Copy(&capturedOutput, r)

	if err != nil {
		t.Fatalf("handleConfigGitLabShow error: %v", err)
	}
	out := capturedOutput.String()
	if strings.Contains(out, "short") {
		t.Fatalf("token must be redacted for short tokens, got: %q", out)
	}
	if !strings.Contains(out, "Token:  ***") {
		t.Fatalf("expected redaction marker, got: %q", out)
	}
}

func TestHandleConfigGitLabShowRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLabShow([]string{"extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleConfigGitLabSetRequiresFile(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLabSet(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleConfigGitLabSetRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLabSet([]string{"--file", "test.json", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

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

	// Set up environment to use the test server.
	t.Setenv("PLOY_CONTROL_PLANE_URL", srv.URL)

	buf := &bytes.Buffer{}
	// Redirect stdout to capture output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err := handleConfigGitLabSet([]string{"--file", configPath}, buf)

	// Close write end and read captured output.
	w.Close()
	var capturedOutput bytes.Buffer
	_, _ = io.Copy(&capturedOutput, r)

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

	output := capturedOutput.String()
	if !strings.Contains(output, "GitLab configuration updated successfully") {
		t.Fatalf("expected success message, got: %q", output)
	}
}

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

func TestHandleConfigGitLabValidateRequiresFile(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLabValidate(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleConfigGitLabValidateRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLabValidate([]string{"--file", "test.json", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

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
	// Redirect stdout to capture output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err := handleConfigGitLabValidate([]string{"--file", configPath}, buf)

	// Close write end and read captured output.
	w.Close()
	var capturedOutput bytes.Buffer
	_, _ = io.Copy(&capturedOutput, r)

	if err != nil {
		t.Fatalf("handleConfigGitLabValidate error: %v", err)
	}

	output := capturedOutput.String()
	if !strings.Contains(output, "GitLab configuration is valid") {
		t.Fatalf("expected validation success message, got: %q", output)
	}
}

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

func TestValidateGitLabConfigURLRules(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *gitLabConfigPayload
		wantErr string
	}{
		{name: "no scheme", cfg: &gitLabConfigPayload{Domain: "gitlab.com", Token: "x"}, wantErr: "domain must use http or https scheme"},
		{name: "ftp scheme", cfg: &gitLabConfigPayload{Domain: "ftp://gitlab.com", Token: "x"}, wantErr: "domain must use http or https scheme"},
		{name: "empty host", cfg: &gitLabConfigPayload{Domain: "https://", Token: "x"}, wantErr: "domain host is required"},
		{name: "http allowed", cfg: &gitLabConfigPayload{Domain: "http://gitlab.local", Token: "x"}, wantErr: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitLabConfig(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

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
	t.Setenv("PLOY_CONTROL_PLANE_URL", srv.URL)

	buf := &bytes.Buffer{}
	err := handleConfigGitLabSet([]string{"--file", configPath}, buf)
	if err == nil || !strings.Contains(err.Error(), "server returned 400: bad") {
		t.Fatalf("expected server error, got: %v", err)
	}
}

func TestValidateGitLabConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *gitLabConfigPayload
		expectErr bool
		errMsg    string
	}{
		{
			name:      "nil config",
			cfg:       nil,
			expectErr: true,
			errMsg:    "configuration is nil",
		},
		{
			name:      "valid config",
			cfg:       &gitLabConfigPayload{Domain: "https://gitlab.com", Token: "glpat-123"},
			expectErr: false,
		},
		{
			name:      "missing domain",
			cfg:       &gitLabConfigPayload{Domain: "", Token: "glpat-123"},
			expectErr: true,
			errMsg:    "domain is required",
		},
		{
			name:      "missing token",
			cfg:       &gitLabConfigPayload{Domain: "https://gitlab.com", Token: ""},
			expectErr: true,
			errMsg:    "token is required",
		},
		{
			name:      "whitespace domain",
			cfg:       &gitLabConfigPayload{Domain: "   ", Token: "glpat-123"},
			expectErr: true,
			errMsg:    "domain is required",
		},
		{
			name:      "whitespace token",
			cfg:       &gitLabConfigPayload{Domain: "https://gitlab.com", Token: "   "},
			expectErr: true,
			errMsg:    "token is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitLabConfig(tt.cfg)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}
