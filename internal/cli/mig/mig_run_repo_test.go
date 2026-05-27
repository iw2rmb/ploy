package mig

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestMigRunRepoRequiredFlagValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		run     func() error
		wantErr string
	}{
		{
			name:    "add without run-id",
			run:     func() error { return RunRunRepoAdd(context.Background(), RunRepoAddOptions{}) },
			wantErr: "run-id required",
		},
		{
			name:    "add without repo-url",
			run:     func() error { return RunRunRepoAdd(context.Background(), RunRepoAddOptions{RunID: "batch-123"}) },
			wantErr: "--repo-url required",
		},
		{
			name: "add without base-ref",
			run: func() error {
				return RunRunRepoAdd(context.Background(), RunRepoAddOptions{RunID: "batch-123", RepoURL: "https://github.com/org/repo.git"})
			},
			wantErr: "--base-ref required",
		},
		{
			name:    "remove without run-id",
			run:     func() error { return RunRunRepoRemove(context.Background(), "", "", nil) },
			wantErr: "run-id required",
		},
		{
			name:    "remove without repo-id",
			run:     func() error { return RunRunRepoRemove(context.Background(), "batch-123", "", nil) },
			wantErr: "--repo-id required",
		},
		{
			name:    "restart without run-id",
			run:     func() error { return RunRunRepoRestart(context.Background(), RunRepoRestartOptions{}) },
			wantErr: "run-id required",
		},
		{
			name: "restart without repo-id",
			run: func() error {
				return RunRunRepoRestart(context.Background(), RunRepoRestartOptions{RunID: "batch-123"})
			},
			wantErr: "--repo-id required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.run()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestMigRunRepoAddCallsControlPlane verifies that `mig run repo add` calls the correct endpoint.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestMigRunRepoAddCallsControlPlane(t *testing.T) {
	var called bool
	var receivedBody map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect POST /v1/runs/{id}/repos
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos" {
			called = true
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			// v1: Use RepoID (mig_repos.id), not a non-existent run_repos.id.
			resp := runRepoResponse{
				RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				RepoID:    "a1b2c3d4", // mig_repos.id (NanoID, 8 chars)
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

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := RunRunRepoAdd(context.Background(), RunRepoAddOptions{
		RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
		RepoURL:   "https://github.com/org/repo.git",
		BaseRef:   "main",
		TargetRef: "feature-branch",
		Output:    buf,
	})
	if err != nil {
		t.Fatalf("mig run repo add error: %v", err)
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

// TestMigRunRepoAddRejectsInvalidRepoURLScheme verifies that the CLI rejects invalid repo_url
// schemes at the input boundary and does not call the control plane.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestMigRunRepoAddRejectsInvalidRepoURLScheme(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	err := RunRunRepoAdd(context.Background(), RunRepoAddOptions{
		RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
		RepoURL:   "http://github.com/org/repo.git",
		BaseRef:   "main",
		TargetRef: "feature-branch",
	})
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

// TestMigRunRepoRemoveCallsControlPlane verifies that `mig run repo remove` calls the correct endpoint.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestMigRunRepoRemoveCallsControlPlane(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect POST /v1/runs/{id}/repos/{repo_id}/cancel
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/cancel" {
			called = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// v1: Use RepoID (mig_repos.id), not a non-existent run_repos.id.
			resp := runRepoResponse{
				RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				RepoID:    "a1b2c3d4", // mig_repos.id (NanoID, 8 chars)
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

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := RunRunRepoRemove(context.Background(), "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep", "a1b2c3d4", buf)
	if err != nil {
		t.Fatalf("mig run repo remove error: %v", err)
	}
	if !called {
		t.Fatal("expected POST /v1/runs/{id}/repos/{repo_id}/cancel to be called")
	}
}

// TestMigRunRepoRestartCallsControlPlane verifies that `mig run repo restart` calls the correct endpoint.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestMigRunRepoRestartCallsControlPlane(t *testing.T) {
	var called bool
	var receivedBody map[string]*string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect POST /v1/runs/{id}/repos/{repo_id}/restart
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/restart" {
			called = true
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// v1: Use RepoID (mig_repos.id), not a non-existent run_repos.id.
			resp := runRepoResponse{
				RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				RepoID:    "a1b2c3d4", // mig_repos.id (NanoID, 8 chars)
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

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := RunRunRepoRestart(context.Background(), RunRepoRestartOptions{
		RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
		RepoID:    "a1b2c3d4",
		TargetRef: "feature-branch-v2",
		Output:    buf,
	})
	if err != nil {
		t.Fatalf("mig run repo restart error: %v", err)
	}
	if !called {
		t.Fatal("expected POST /v1/runs/{id}/repos/{repo_id}/restart to be called")
	}
	// Verify optional target-ref was sent.
	if receivedBody["target_ref"] == nil || *receivedBody["target_ref"] != "feature-branch-v2" {
		t.Errorf("expected target_ref=feature-branch-v2 in request body")
	}
}

// TestMigRunRepoAddServerError verifies error handling when the server returns an error.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestMigRunRepoAddServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos" {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := RunRunRepoAdd(context.Background(), RunRepoAddOptions{
		RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
		RepoURL:   "https://github.com/org/repo.git",
		BaseRef:   "main",
		TargetRef: "feature-branch",
		Output:    buf,
	})
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

// TestMigRunRepoRemoveServerError verifies error handling when remove fails.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestMigRunRepoRemoveServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/cancel" {
			http.Error(w, "repo not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := RunRunRepoRemove(context.Background(), "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep", "a1b2c3d4", buf)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error mentioning 404 or not found, got: %v", err)
	}
}

// TestMigRunRepoRestartServerError verifies error handling when restart fails.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestMigRunRepoRestartServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/restart" {
			// Conflict: cannot restart a non-terminal repo.
			http.Error(w, "can only restart repos in terminal state", http.StatusConflict)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := RunRunRepoRestart(context.Background(), RunRepoRestartOptions{
		RunID:  "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
		RepoID: "a1b2c3d4",
		Output: buf,
	})
	if err == nil {
		t.Fatal("expected error for 409 response")
	}
	if !strings.Contains(err.Error(), "409") && !strings.Contains(err.Error(), "terminal") {
		t.Errorf("expected error mentioning 409 or terminal, got: %v", err)
	}
}

// TestMigRunRepoRestartWithBaseRef verifies restart sends optional base-ref.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestMigRunRepoRestartWithBaseRef(t *testing.T) {
	var receivedBody map[string]*string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/restart" {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// v1: Use RepoID (mig_repos.id), not a non-existent run_repos.id.
			resp := runRepoResponse{
				RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				RepoID:    "a1b2c3d4", // mig_repos.id (NanoID, 8 chars)
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

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := RunRunRepoRestart(context.Background(), RunRepoRestartOptions{
		RunID:   "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
		RepoID:  "a1b2c3d4",
		BaseRef: "main-v2",
		Output:  buf,
	})
	if err != nil {
		t.Fatalf("mig run repo restart error: %v", err)
	}

	// Verify base-ref was sent in request body.
	if receivedBody["base_ref"] == nil || *receivedBody["base_ref"] != "main-v2" {
		t.Errorf("expected base_ref=main-v2 in request body, got %v", receivedBody)
	}
}

// TestMigRunRepoRestartWithBothRefs verifies restart sends both base and target refs.
// Note: Not parallel because useServerDescriptor uses t.Setenv.
func TestMigRunRepoRestartWithBothRefs(t *testing.T) {
	var receivedBody map[string]*string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/2HBZ1MRFOo8uvXVJhVqKlf8W8Ep/repos/a1b2c3d4/restart" {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// v1: Use RepoID (mig_repos.id), not a non-existent run_repos.id.
			resp := runRepoResponse{
				RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				RepoID:    "a1b2c3d4", // mig_repos.id (NanoID, 8 chars)
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

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := RunRunRepoRestart(context.Background(), RunRepoRestartOptions{
		RunID:     "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
		RepoID:    "a1b2c3d4",
		BaseRef:   "main-v2",
		TargetRef: "feature-v2",
		Output:    buf,
	})
	if err != nil {
		t.Fatalf("mig run repo restart error: %v", err)
	}

	// Verify both refs were sent in request body.
	if receivedBody["base_ref"] == nil || *receivedBody["base_ref"] != "main-v2" {
		t.Errorf("expected base_ref=main-v2 in request body, got %v", receivedBody)
	}
	if receivedBody["target_ref"] == nil || *receivedBody["target_ref"] != "feature-v2" {
		t.Errorf("expected target_ref=feature-v2 in request body, got %v", receivedBody)
	}
}
