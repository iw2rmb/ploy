package mods

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// =============================================================================
// RunPullCommand Tests
// =============================================================================

// TestRunPullCommand_Success verifies successful pull resolution for a run.
func TestRunPullCommand_Success(t *testing.T) {
	t.Parallel()

	// Create a mock server that returns a valid response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/runs/") || !strings.HasSuffix(r.URL.Path, "/pull") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Verify content type.
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		// Parse and verify request body.
		var req struct {
			RepoURL string `json:"repo_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if req.RepoURL == "" {
			t.Error("expected non-empty repo_url in request")
		}

		// Return a valid response.
		resp := PullResolution{
			RunID:         "run123",
			RepoID:        "repo456",
			RepoTargetRef: "mods/run123/feature",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	baseURL, _ := url.Parse(server.URL)

	cmd := RunPullCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		RunID:   "run123",
		RepoURL: "https://github.com/example/repo.git",
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RunID != "run123" {
		t.Errorf("expected RunID 'run123', got %q", result.RunID)
	}
	if result.RepoID != "repo456" {
		t.Errorf("expected RepoID 'repo456', got %q", result.RepoID)
	}
	if result.RepoTargetRef != "mods/run123/feature" {
		t.Errorf("expected RepoTargetRef 'mods/run123/feature', got %q", result.RepoTargetRef)
	}
}

// TestRunPullCommand_NotFound verifies error handling for 404 responses.
func TestRunPullCommand_NotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "run not found", http.StatusNotFound)
	}))
	defer server.Close()

	baseURL, _ := url.Parse(server.URL)

	cmd := RunPullCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		RunID:   "nonexistent",
		RepoURL: "https://github.com/example/repo.git",
	}

	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "run pull") {
		t.Errorf("error should contain 'run pull', got: %v", err)
	}
}

// TestRunPullCommand_ValidationErrors verifies input validation.
func TestRunPullCommand_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     RunPullCommand
		wantErr string
	}{
		{
			name:    "nil client",
			cmd:     RunPullCommand{RunID: "run123", RepoURL: "https://example.com"},
			wantErr: "http client required",
		},
		{
			name: "nil base url",
			cmd: RunPullCommand{
				Client:  http.DefaultClient,
				RunID:   "run123",
				RepoURL: "https://example.com",
			},
			wantErr: "base url required",
		},
		{
			name: "empty run id",
			cmd: RunPullCommand{
				Client:  http.DefaultClient,
				BaseURL: &url.URL{Scheme: "http", Host: "localhost"},
				RepoURL: "https://example.com",
			},
			wantErr: "run id required",
		},
		{
			name: "empty repo url",
			cmd: RunPullCommand{
				Client:  http.DefaultClient,
				BaseURL: &url.URL{Scheme: "http", Host: "localhost"},
				RunID:   "run123",
			},
			wantErr: "repo url required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := tc.cmd.Run(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error should contain %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// =============================================================================
// ModPullCommand Tests
// =============================================================================

// TestModPullCommand_Success verifies successful pull resolution for a mod.
func TestModPullCommand_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/mods/") || !strings.HasSuffix(r.URL.Path, "/pull") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Parse and verify request body.
		var req struct {
			RepoURL string `json:"repo_url"`
			Mode    string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if req.RepoURL == "" {
			t.Error("expected non-empty repo_url in request")
		}

		// Return a valid response.
		resp := PullResolution{
			RunID:         "run789",
			RepoID:        "repo101",
			RepoTargetRef: "mods/run789/feature",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	baseURL, _ := url.Parse(server.URL)

	cmd := ModPullCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		ModID:   "my-mod",
		RepoURL: "https://github.com/example/repo.git",
		Mode:    PullModeLastSucceeded,
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RunID != "run789" {
		t.Errorf("expected RunID 'run789', got %q", result.RunID)
	}
	if result.RepoID != "repo101" {
		t.Errorf("expected RepoID 'repo101', got %q", result.RepoID)
	}
}

// TestModPullCommand_WithLastFailed verifies that last-failed mode is sent correctly.
func TestModPullCommand_WithLastFailed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body.
		var req struct {
			RepoURL string `json:"repo_url"`
			Mode    string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// Verify mode is set to last-failed.
		if req.Mode != "last-failed" {
			t.Errorf("expected mode 'last-failed', got %q", req.Mode)
		}

		// Return a valid response.
		resp := PullResolution{
			RunID:         "run-failed",
			RepoID:        "repo-failed",
			RepoTargetRef: "mods/run-failed/fix",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	baseURL, _ := url.Parse(server.URL)

	cmd := ModPullCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		ModID:   "my-mod",
		RepoURL: "https://github.com/example/repo.git",
		Mode:    PullModeLastFailed,
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RunID != "run-failed" {
		t.Errorf("expected RunID 'run-failed', got %q", result.RunID)
	}
}

// TestModPullCommand_DefaultMode verifies that mode defaults to last-succeeded.
func TestModPullCommand_DefaultMode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body.
		var req struct {
			RepoURL string `json:"repo_url"`
			Mode    string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// Verify mode defaults to last-succeeded.
		if req.Mode != "last-succeeded" {
			t.Errorf("expected mode 'last-succeeded' (default), got %q", req.Mode)
		}

		// Return a valid response.
		resp := PullResolution{
			RunID:         "run-default",
			RepoID:        "repo-default",
			RepoTargetRef: "mods/run-default/feature",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	baseURL, _ := url.Parse(server.URL)

	cmd := ModPullCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		ModID:   "my-mod",
		RepoURL: "https://github.com/example/repo.git",
		// Mode is intentionally not set to test default behavior.
	}

	_, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestModPullCommand_ValidationErrors verifies input validation.
func TestModPullCommand_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     ModPullCommand
		wantErr string
	}{
		{
			name:    "nil client",
			cmd:     ModPullCommand{ModID: "my-mod", RepoURL: "https://example.com"},
			wantErr: "http client required",
		},
		{
			name: "nil base url",
			cmd: ModPullCommand{
				Client:  http.DefaultClient,
				ModID:   "my-mod",
				RepoURL: "https://example.com",
			},
			wantErr: "base url required",
		},
		{
			name: "empty mod id",
			cmd: ModPullCommand{
				Client:  http.DefaultClient,
				BaseURL: &url.URL{Scheme: "http", Host: "localhost"},
				RepoURL: "https://example.com",
			},
			wantErr: "mod id required",
		},
		{
			name: "empty repo url",
			cmd: ModPullCommand{
				Client:  http.DefaultClient,
				BaseURL: &url.URL{Scheme: "http", Host: "localhost"},
				ModID:   "my-mod",
			},
			wantErr: "repo url required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := tc.cmd.Run(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error should contain %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// TestModPullCommand_NotFound verifies error handling for 404 responses.
func TestModPullCommand_NotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "mod not found", http.StatusNotFound)
	}))
	defer server.Close()

	baseURL, _ := url.Parse(server.URL)

	cmd := ModPullCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		ModID:   "nonexistent",
		RepoURL: "https://github.com/example/repo.git",
	}

	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "mod pull") {
		t.Errorf("error should contain 'mod pull', got: %v", err)
	}
}

// =============================================================================
// PullMode Constants Tests
// =============================================================================

// TestPullModeConstants verifies that the pull mode constants have expected values.
func TestPullModeConstants(t *testing.T) {
	t.Parallel()

	if PullModeLastSucceeded != "last-succeeded" {
		t.Errorf("expected PullModeLastSucceeded to be 'last-succeeded', got %q", PullModeLastSucceeded)
	}
	if PullModeLastFailed != "last-failed" {
		t.Errorf("expected PullModeLastFailed to be 'last-failed', got %q", PullModeLastFailed)
	}
}
