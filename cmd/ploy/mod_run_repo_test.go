package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestModRunRepoRouting verifies that `mod run repo` dispatches to the correct handler.
// Tests argument parsing without making HTTP calls (no t.Setenv, so t.Parallel is safe).
func TestModRunRepoRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "no action shows usage",
			args:    []string{"mod", "run", "repo"},
			wantErr: "mod run repo action required",
		},
		{
			name:    "unknown action",
			args:    []string{"mod", "run", "repo", "unknown"},
			wantErr: `unknown mod run repo action "unknown"`,
		},
		{
			name:    "add without batch-id",
			args:    []string{"mod", "run", "repo", "add"},
			wantErr: "batch-id required",
		},
		{
			name:    "add without repo-url",
			args:    []string{"mod", "run", "repo", "add", "batch-123"},
			wantErr: "--repo-url required",
		},
		{
			name:    "add without base-ref",
			args:    []string{"mod", "run", "repo", "add", "--repo-url", "https://github.com/org/repo.git", "batch-123"},
			wantErr: "--base-ref required",
		},
		{
			name:    "add without target-ref",
			args:    []string{"mod", "run", "repo", "add", "--repo-url", "https://github.com/org/repo.git", "--base-ref", "main", "batch-123"},
			wantErr: "--target-ref required",
		},
		{
			name:    "remove without batch-id",
			args:    []string{"mod", "run", "repo", "remove"},
			wantErr: "batch-id required",
		},
		{
			name:    "remove without repo-id",
			args:    []string{"mod", "run", "repo", "remove", "batch-123"},
			wantErr: "--repo-id required",
		},
		{
			name:    "restart without batch-id",
			args:    []string{"mod", "run", "repo", "restart"},
			wantErr: "batch-id required",
		},
		{
			name:    "restart without repo-id",
			args:    []string{"mod", "run", "repo", "restart", "batch-123"},
			wantErr: "--repo-id required",
		},
		{
			name:    "status without batch-id",
			args:    []string{"mod", "run", "repo", "status"},
			wantErr: "batch-id required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			buf := &bytes.Buffer{}
			err := execute(tc.args, buf)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

// TestModRunRepoAddCallsControlPlane verifies that `mod run repo add` calls the correct endpoint.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoAddCallsControlPlane(t *testing.T) {
	var called bool
	var receivedBody map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect POST /v1/runs/{id}/repos
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/batch-uuid-123/repos" {
			called = true
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			resp := runRepoResponse{
				ID:        "repo-uuid-456",
				RunID:     "batch-uuid-123",
				RepoURL:   receivedBody["repo_url"],
				BaseRef:   receivedBody["base_ref"],
				TargetRef: receivedBody["target_ref"],
				Status:    "pending",
				Attempt:   1,
				CreatedAt: time.Now(),
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	// Note: Flags must come before the positional batch-id argument for flag parsing.
	err := execute([]string{
		"mod", "run", "repo", "add",
		"--repo-url", "https://github.com/org/repo.git",
		"--base-ref", "main",
		"--target-ref", "feature-branch",
		"batch-uuid-123",
	}, buf)
	if err != nil {
		t.Fatalf("mod run repo add error: %v", err)
	}
	if !called {
		t.Fatal("expected POST /v1/runs/{id}/repos to be called")
	}
	if receivedBody["repo_url"] != "https://github.com/org/repo.git" {
		t.Errorf("expected repo_url=https://github.com/org/repo.git, got %s", receivedBody["repo_url"])
	}
	if receivedBody["base_ref"] != "main" {
		t.Errorf("expected base_ref=main, got %s", receivedBody["base_ref"])
	}
	if receivedBody["target_ref"] != "feature-branch" {
		t.Errorf("expected target_ref=feature-branch, got %s", receivedBody["target_ref"])
	}
}

// TestModRunRepoRemoveCallsControlPlane verifies that `mod run repo remove` calls the correct endpoint.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoRemoveCallsControlPlane(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect DELETE /v1/runs/{id}/repos/{repo_id}
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/runs/batch-uuid-123/repos/repo-uuid-456" {
			called = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := runRepoResponse{
				ID:        "repo-uuid-456",
				RunID:     "batch-uuid-123",
				RepoURL:   "https://github.com/org/repo.git",
				BaseRef:   "main",
				TargetRef: "feature-branch",
				Status:    "skipped",
				Attempt:   1,
				CreatedAt: time.Now(),
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	// Note: Flags must come before the positional batch-id argument for flag parsing.
	err := execute([]string{
		"mod", "run", "repo", "remove",
		"--repo-id", "repo-uuid-456",
		"batch-uuid-123",
	}, buf)
	if err != nil {
		t.Fatalf("mod run repo remove error: %v", err)
	}
	if !called {
		t.Fatal("expected DELETE /v1/runs/{id}/repos/{repo_id} to be called")
	}
}

// TestModRunRepoRestartCallsControlPlane verifies that `mod run repo restart` calls the correct endpoint.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoRestartCallsControlPlane(t *testing.T) {
	var called bool
	var receivedBody map[string]*string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect POST /v1/runs/{id}/repos/{repo_id}/restart
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/batch-uuid-123/repos/repo-uuid-456/restart" {
			called = true
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := runRepoResponse{
				ID:        "repo-uuid-456",
				RunID:     "batch-uuid-123",
				RepoURL:   "https://github.com/org/repo.git",
				BaseRef:   "main",
				TargetRef: "feature-branch-v2",
				Status:    "pending",
				Attempt:   2,
				CreatedAt: time.Now(),
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	// Note: Flags must come before the positional batch-id argument for flag parsing.
	err := execute([]string{
		"mod", "run", "repo", "restart",
		"--repo-id", "repo-uuid-456",
		"--target-ref", "feature-branch-v2",
		"batch-uuid-123",
	}, buf)
	if err != nil {
		t.Fatalf("mod run repo restart error: %v", err)
	}
	if !called {
		t.Fatal("expected POST /v1/runs/{id}/repos/{repo_id}/restart to be called")
	}
	// Verify optional target-ref was sent.
	if receivedBody["target_ref"] == nil || *receivedBody["target_ref"] != "feature-branch-v2" {
		t.Errorf("expected target_ref=feature-branch-v2 in request body")
	}
}

// TestModRunRepoStatusCallsControlPlane verifies that `mod run repo status` calls the correct endpoint.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoStatusCallsControlPlane(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect GET /v1/runs/{id}/repos
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/batch-uuid-123/repos" {
			called = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			repos := []runRepoResponse{
				{
					ID:        "repo-uuid-1",
					RunID:     "batch-uuid-123",
					RepoURL:   "https://github.com/org/repo1.git",
					BaseRef:   "main",
					TargetRef: "feature-1",
					Status:    "succeeded",
					Attempt:   1,
					CreatedAt: time.Now(),
				},
				{
					ID:        "repo-uuid-2",
					RunID:     "batch-uuid-123",
					RepoURL:   "https://github.com/org/repo2.git",
					BaseRef:   "main",
					TargetRef: "feature-2",
					Status:    "pending",
					Attempt:   1,
					CreatedAt: time.Now(),
				},
			}
			resp := struct {
				Repos []runRepoResponse `json:"repos"`
			}{Repos: repos}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "run", "repo", "status", "batch-uuid-123"}, buf)
	if err != nil {
		t.Fatalf("mod run repo status error: %v", err)
	}
	if !called {
		t.Fatal("expected GET /v1/runs/{id}/repos to be called")
	}
}

// TestModRunRepoStatusEmptyBatch verifies that `mod run repo status` handles empty batches.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoStatusEmptyBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/empty-batch/repos" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := struct {
				Repos []runRepoResponse `json:"repos"`
			}{Repos: []runRepoResponse{}}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "run", "repo", "status", "empty-batch"}, buf)
	if err != nil {
		t.Fatalf("mod run repo status error: %v", err)
	}
	// Should print "No repos found in this batch."
	if !strings.Contains(buf.String(), "No repos found") {
		t.Errorf("expected 'No repos found' message, got: %s", buf.String())
	}
}

// TestModRunRepoAddServerError verifies error handling when the server returns an error.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoAddServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/batch-uuid-123/repos" {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	// Note: Flags must come before the positional batch-id argument for flag parsing.
	err := execute([]string{
		"mod", "run", "repo", "add",
		"--repo-url", "https://github.com/org/repo.git",
		"--base-ref", "main",
		"--target-ref", "feature-branch",
		"batch-uuid-123",
	}, buf)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error mentioning 404 or not found, got: %v", err)
	}
}
