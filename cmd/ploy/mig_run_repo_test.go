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
			args:    []string{"mig", "run", "repo"},
			wantErr: "mod run repo action required",
		},
		{
			name:    "unknown action",
			args:    []string{"mig", "run", "repo", "unknown"},
			wantErr: `unknown mod run repo action "unknown"`,
		},
		{
			name:    "add without run-id",
			args:    []string{"mig", "run", "repo", "add"},
			wantErr: "run-id required",
		},
		{
			name:    "add without repo-url",
			args:    []string{"mig", "run", "repo", "add", "batch-123"},
			wantErr: "--repo-url required",
		},
		{
			name:    "add without base-ref",
			args:    []string{"mig", "run", "repo", "add", "--repo-url", "https://github.com/org/repo.git", "batch-123"},
			wantErr: "--base-ref required",
		},
		{
			name:    "remove without run-id",
			args:    []string{"mig", "run", "repo", "remove"},
			wantErr: "run-id required",
		},
		{
			name:    "remove without repo-id",
			args:    []string{"mig", "run", "repo", "remove", "batch-123"},
			wantErr: "--repo-id required",
		},
		{
			name:    "restart without run-id",
			args:    []string{"mig", "run", "repo", "restart"},
			wantErr: "run-id required",
		},
		{
			name:    "restart without repo-id",
			args:    []string{"mig", "run", "repo", "restart", "batch-123"},
			wantErr: "--repo-id required",
		},
		{
			name:    "status without run-id",
			args:    []string{"mig", "run", "repo", "status"},
			wantErr: "run-id required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			buf := &bytes.Buffer{}
			err := executeCmd(tc.args, buf)
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
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos" {
			called = true
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			// v1: Use RepoID (mod_repos.id), not a non-existent run_repos.id.
			resp := runRepoResponse{
				RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				RepoID:    "a1b2c3d4", // mod_repos.id (NanoID, 8 chars)
				RepoURL:   receivedBody["repo_url"],
				BaseRef:   receivedBody["base_ref"],
				TargetRef: receivedBody["target_ref"],
				Status:    "Queued",
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
	// Note: Flags must come before the positional run-id argument for flag parsing.
	err := executeCmd([]string{
		"mig", "run", "repo", "add",
		"--repo-url", "https://github.com/org/repo.git",
		"--base-ref", "main",
		"--target-ref", "feature-branch",
		"2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
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

// TestModRunRepoAddRejectsInvalidRepoURLScheme verifies that the CLI rejects invalid repo_url
// schemes at the input boundary and does not call the control plane.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoAddRejectsInvalidRepoURLScheme(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{
		"mig", "run", "repo", "add",
		"--repo-url", "http://github.com/org/repo.git",
		"--base-ref", "main",
		"--target-ref", "feature-branch",
		"2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
	}, buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--repo-url") {
		t.Fatalf("expected error to mention --repo-url, got %q", err.Error())
	}
	if called {
		t.Fatal("expected no control plane request for invalid repo URL scheme")
	}
}

// TestModRunRepoRemoveCallsControlPlane verifies that `mod run repo remove` calls the correct endpoint.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoRemoveCallsControlPlane(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect POST /v1/runs/{id}/repos/{repo_id}/cancel
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/cancel" {
			called = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// v1: Use RepoID (mod_repos.id), not a non-existent run_repos.id.
			resp := runRepoResponse{
				RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				RepoID:    "a1b2c3d4", // mod_repos.id (NanoID, 8 chars)
				RepoURL:   "https://github.com/org/repo.git",
				BaseRef:   "main",
				TargetRef: "feature-branch",
				Status:    "Cancelled",
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
	// Note: Flags must come before the positional run-id argument for flag parsing.
	err := executeCmd([]string{
		"mig", "run", "repo", "remove",
		"--repo-id", "a1b2c3d4",
		"2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
	}, buf)
	if err != nil {
		t.Fatalf("mod run repo remove error: %v", err)
	}
	if !called {
		t.Fatal("expected POST /v1/runs/{id}/repos/{repo_id}/cancel to be called")
	}
}

// TestModRunRepoRestartCallsControlPlane verifies that `mod run repo restart` calls the correct endpoint.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoRestartCallsControlPlane(t *testing.T) {
	var called bool
	var receivedBody map[string]*string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect POST /v1/runs/{id}/repos/{repo_id}/restart
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/restart" {
			called = true
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// v1: Use RepoID (mod_repos.id), not a non-existent run_repos.id.
			resp := runRepoResponse{
				RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				RepoID:    "a1b2c3d4", // mod_repos.id (NanoID, 8 chars)
				RepoURL:   "https://github.com/org/repo.git",
				BaseRef:   "main",
				TargetRef: "feature-branch-v2",
				Status:    "Queued",
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
	// Note: Flags must come before the positional run-id argument for flag parsing.
	err := executeCmd([]string{
		"mig", "run", "repo", "restart",
		"--repo-id", "a1b2c3d4",
		"--target-ref", "feature-branch-v2",
		"2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
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
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos" {
			called = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// v1: Use RepoID (mod_repos.id), not a non-existent run_repos.id.
			repos := []runRepoResponse{
				{
					RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
					RepoID:    "e5f6g7h8", // mod_repos.id (NanoID, 8 chars)
					RepoURL:   "https://github.com/org/repo1.git",
					BaseRef:   "main",
					TargetRef: "feature-1",
					Status:    "Success",
					Attempt:   1,
					CreatedAt: time.Now(),
				},
				{
					RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
					RepoID:    "i9j0k1l2", // mod_repos.id (NanoID, 8 chars)
					RepoURL:   "https://github.com/org/repo2.git",
					BaseRef:   "main",
					TargetRef: "feature-2",
					Status:    "Queued",
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
	err := executeCmd([]string{"mig", "run", "repo", "status", "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep"}, buf)
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
	err := executeCmd([]string{"mig", "run", "repo", "status", "empty-batch"}, buf)
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
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos" {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	// Note: Flags must come before the positional run-id argument for flag parsing.
	err := executeCmd([]string{
		"mig", "run", "repo", "add",
		"--repo-url", "https://github.com/org/repo.git",
		"--base-ref", "main",
		"--target-ref", "feature-branch",
		"2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
	}, buf)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error mentioning 404 or not found, got: %v", err)
	}
}

// =========================================================================
// Focused batch run workflow CLI tests:
// Verifies that CLI subcommands validate arguments and call correct endpoints.
// =========================================================================

// TestModRunRepoRemoveServerError verifies error handling when remove fails.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoRemoveServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/cancel" {
			http.Error(w, "repo not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{
		"mig", "run", "repo", "remove",
		"--repo-id", "a1b2c3d4",
		"2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
	}, buf)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error mentioning 404 or not found, got: %v", err)
	}
}

// TestModRunRepoRestartServerError verifies error handling when restart fails.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoRestartServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/restart" {
			// Conflict: cannot restart a non-terminal repo.
			http.Error(w, "can only restart repos in terminal state", http.StatusConflict)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{
		"mig", "run", "repo", "restart",
		"--repo-id", "a1b2c3d4",
		"2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
	}, buf)
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "409") && !strings.Contains(err.Error(), "terminal") {
		t.Errorf("expected error mentioning 409 or terminal, got: %v", err)
	}
}

// TestModRunRepoStatusServerError verifies error handling when status query fails.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoStatusServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/unknown-batch/repos" {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"mig", "run", "repo", "status", "unknown-batch"}, buf)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error mentioning 404 or not found, got: %v", err)
	}
}

// TestModRunRepoRestartWithBaseRef verifies restart sends optional base-ref.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoRestartWithBaseRef(t *testing.T) {
	var receivedBody map[string]*string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/restart" {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// v1: Use RepoID (mod_repos.id), not a non-existent run_repos.id.
			resp := runRepoResponse{
				RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				RepoID:    "a1b2c3d4", // mod_repos.id (NanoID, 8 chars)
				RepoURL:   "https://github.com/org/repo.git",
				BaseRef:   "main-v2",
				TargetRef: "feature-branch",
				Status:    "Queued",
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
	err := executeCmd([]string{
		"mig", "run", "repo", "restart",
		"--repo-id", "a1b2c3d4",
		"--base-ref", "main-v2",
		"2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
	}, buf)
	if err != nil {
		t.Fatalf("mod run repo restart error: %v", err)
	}

	// Verify base-ref was sent in request body.
	if receivedBody["base_ref"] == nil || *receivedBody["base_ref"] != "main-v2" {
		t.Errorf("expected base_ref=main-v2 in request body, got %v", receivedBody)
	}
}

// TestModRunRepoRestartWithBothRefs verifies restart sends both base and target refs.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunRepoRestartWithBothRefs(t *testing.T) {
	var receivedBody map[string]*string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/restart" {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// v1: Use RepoID (mod_repos.id), not a non-existent run_repos.id.
			resp := runRepoResponse{
				RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				RepoID:    "a1b2c3d4", // mod_repos.id (NanoID, 8 chars)
				RepoURL:   "https://github.com/org/repo.git",
				BaseRef:   "main-v2",
				TargetRef: "feature-v2",
				Status:    "Queued",
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
	err := executeCmd([]string{
		"mig", "run", "repo", "restart",
		"--repo-id", "a1b2c3d4",
		"--base-ref", "main-v2",
		"--target-ref", "feature-v2",
		"2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
	}, buf)
	if err != nil {
		t.Fatalf("mod run repo restart error: %v", err)
	}

	// Verify both refs were sent in request body.
	if receivedBody["base_ref"] == nil || *receivedBody["base_ref"] != "main-v2" {
		t.Errorf("expected base_ref=main-v2 in request body, got %v", receivedBody)
	}
	if receivedBody["target_ref"] == nil || *receivedBody["target_ref"] != "feature-v2" {
		t.Errorf("expected target_ref=feature-v2 in request body, got %v", receivedBody)
	}
}
