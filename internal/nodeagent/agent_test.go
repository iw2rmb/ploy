package nodeagent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestRunControllerStartRun(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	req := StartRunRequest{
		RunID:   "run-001",
		RepoURL: "https://github.com/example/repo.git",
		BaseRef: "main",
	}

	ctx := context.Background()
	if err := rc.StartRun(ctx, req); err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	if _, exists := rc.runs[req.RunID]; !exists {
		t.Errorf("run %s not found in controller", req.RunID)
	}
}

func TestRunControllerStartRunDuplicate(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	req := StartRunRequest{
		RunID:   "run-001",
		RepoURL: "https://github.com/example/repo.git",
		BaseRef: "main",
	}

	ctx := context.Background()

	// Start the run once.
	if err := rc.StartRun(ctx, req); err != nil {
		t.Fatalf("first StartRun() error = %v", err)
	}

	// Try to start the same run again.
	err := rc.StartRun(ctx, req)
	if err == nil {
		t.Errorf("expected error for duplicate run, got nil")
	}
}

func TestRunControllerStopRun(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	// Start a run first.
	startReq := StartRunRequest{
		RunID:   "run-001",
		RepoURL: "https://github.com/example/repo.git",
		BaseRef: "main",
	}

	ctx := context.Background()
	if err := rc.StartRun(ctx, startReq); err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	// Stop the run.
	stopReq := StopRunRequest{
		RunID:  "run-001",
		Reason: "test",
	}

	if err := rc.StopRun(ctx, stopReq); err != nil {
		t.Errorf("StopRun() error = %v", err)
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	if _, exists := rc.runs[stopReq.RunID]; exists {
		t.Errorf("run %s still exists after stop", stopReq.RunID)
	}
}

func TestRunControllerStopNonExistent(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	stopReq := StopRunRequest{
		RunID:  "nonexistent",
		Reason: "test",
	}

	ctx := context.Background()
	err := rc.StopRun(ctx, stopReq)
	if err == nil {
		t.Errorf("expected error for nonexistent run, got nil")
	}
	if err.Error() != fmt.Sprintf("run %s not found", stopReq.RunID) {
		t.Errorf("error = %v, want 'run %s not found'", err, stopReq.RunID)
	}
}

func TestBuildManifestFromRequest(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		req := StartRunRequest{
			RunID:     "run-123",
			RepoURL:   "https://github.com/example/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CommitSHA: "abc123",
			Env: map[string]string{
				"FOO": "bar",
			},
		}

		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if manifest.ID != req.RunID {
			t.Errorf("expected ID %q, got %q", req.RunID, manifest.ID)
		}
		if manifest.Image != "ubuntu:latest" {
			t.Errorf("expected default image ubuntu:latest, got %q", manifest.Image)
		}
		if manifest.WorkingDir != "/workspace" {
			t.Errorf("expected working dir /workspace, got %q", manifest.WorkingDir)
		}
		if len(manifest.Inputs) != 1 {
			t.Fatalf("expected 1 input, got %d", len(manifest.Inputs))
		}

		input := manifest.Inputs[0]
		if input.Name != "workspace" {
			t.Errorf("expected input name workspace, got %q", input.Name)
		}
		if input.MountPath != "/workspace" {
			t.Errorf("expected mount path /workspace, got %q", input.MountPath)
		}
		if input.Mode != contracts.StepInputModeReadWrite {
			t.Errorf("expected read-write mode, got %q", input.Mode)
		}
		if input.Hydration == nil {
			t.Fatal("expected hydration to be set")
		}
		if input.Hydration.Repo == nil {
			t.Fatal("expected repo to be set in hydration")
		}

		repo := input.Hydration.Repo
		if repo.URL != req.RepoURL {
			t.Errorf("expected repo URL %q, got %q", req.RepoURL, repo.URL)
		}
		if repo.BaseRef != req.BaseRef {
			t.Errorf("expected base ref %q, got %q", req.BaseRef, repo.BaseRef)
		}
		if repo.TargetRef != req.TargetRef {
			t.Errorf("expected target ref %q, got %q", req.TargetRef, repo.TargetRef)
		}
		if repo.Commit != req.CommitSHA {
			t.Errorf("expected commit %q, got %q", req.CommitSHA, repo.Commit)
		}

		if len(manifest.Env) != 1 {
			t.Errorf("expected 1 env var, got %d", len(manifest.Env))
		}
		if manifest.Env["FOO"] != "bar" {
			t.Errorf("expected env FOO=bar, got %q", manifest.Env["FOO"])
		}
	})

	t.Run("missing run_id", func(t *testing.T) {
		req := StartRunRequest{
			RepoURL: "https://github.com/example/repo.git",
		}

		_, err := buildManifestFromRequest(req)
		if err == nil {
			t.Fatal("expected error for missing run_id")
		}
		if !strings.Contains(err.Error(), "run_id required") {
			t.Errorf("expected error about run_id, got %v", err)
		}
	})

	t.Run("missing repo_url", func(t *testing.T) {
		req := StartRunRequest{
			RunID: "run-123",
		}

		_, err := buildManifestFromRequest(req)
		if err == nil {
			t.Fatal("expected error for missing repo_url")
		}
		if !strings.Contains(err.Error(), "repo_url required") {
			t.Errorf("expected error about repo_url, got %v", err)
		}
	})

	t.Run("defaults target_ref from base_ref", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   "run-123",
			RepoURL: "https://github.com/example/repo.git",
			BaseRef: "main",
		}

		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if manifest.Inputs[0].Hydration.Repo.TargetRef != "main" {
			t.Errorf("expected target_ref to default to main, got %q", manifest.Inputs[0].Hydration.Repo.TargetRef)
		}
	})

	t.Run("validates manifest", func(t *testing.T) {
		req := StartRunRequest{
			RunID:     "run-123",
			RepoURL:   "https://github.com/example/repo.git",
			TargetRef: "main",
		}

		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if err := manifest.Validate(); err != nil {
			t.Errorf("manifest validation failed: %v", err)
		}
	})

	// Accept command as either []string or single string.
	t.Run("command option string maps to shell", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   "run-123",
			RepoURL: "https://github.com/example/repo.git",
			Options: map[string]any{
				"command": "echo hi",
			},
		}

		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}
		want := []string{"/bin/sh", "-c", "echo hi"}
		if len(manifest.Command) != len(want) {
			t.Fatalf("command len=%d, want %d", len(manifest.Command), len(want))
		}
		for i := range want {
			if manifest.Command[i] != want[i] {
				t.Fatalf("command[%d]=%q, want %q", i, manifest.Command[i], want[i])
			}
		}
	})
}

func TestWorkspaceLifecycle(t *testing.T) {
	t.Run("workspace is created with unique prefix", func(t *testing.T) {
		// Create two workspaces and verify they have unique paths.
		ws1, err := createEphemeralWorkspace()
		if err != nil {
			t.Fatalf("failed to create first workspace: %v", err)
		}
		defer cleanupWorkspace(ws1)

		ws2, err := createEphemeralWorkspace()
		if err != nil {
			t.Fatalf("failed to create second workspace: %v", err)
		}
		defer cleanupWorkspace(ws2)

		if ws1 == ws2 {
			t.Errorf("expected unique workspace paths, got %q == %q", ws1, ws2)
		}

		// Verify prefix is correct.
		if !strings.Contains(ws1, "ploy-run-") {
			t.Errorf("expected workspace path to contain 'ploy-run-', got %q", ws1)
		}
		if !strings.Contains(ws2, "ploy-run-") {
			t.Errorf("expected workspace path to contain 'ploy-run-', got %q", ws2)
		}
	})

	t.Run("workspace directory exists after creation", func(t *testing.T) {
		ws, err := createEphemeralWorkspace()
		if err != nil {
			t.Fatalf("failed to create workspace: %v", err)
		}
		defer cleanupWorkspace(ws)

		info, err := os.Stat(ws)
		if err != nil {
			t.Fatalf("workspace directory does not exist: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("workspace path %q is not a directory", ws)
		}
	})

	t.Run("workspace cleanup removes directory", func(t *testing.T) {
		ws, err := createEphemeralWorkspace()
		if err != nil {
			t.Fatalf("failed to create workspace: %v", err)
		}

		// Verify it exists.
		if _, err := os.Stat(ws); err != nil {
			t.Fatalf("workspace should exist before cleanup: %v", err)
		}

		// Cleanup.
		cleanupWorkspace(ws)

		// Verify it no longer exists.
		if _, err := os.Stat(ws); err == nil {
			t.Errorf("workspace %q should not exist after cleanup", ws)
		}
	})

	t.Run("workspace cleanup removes nested content", func(t *testing.T) {
		ws, err := createEphemeralWorkspace()
		if err != nil {
			t.Fatalf("failed to create workspace: %v", err)
		}

		// Create nested files and directories.
		testFile := fmt.Sprintf("%s/test.txt", ws)
		if err := os.WriteFile(testFile, []byte("test content"), 0o600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		nestedDir := fmt.Sprintf("%s/nested", ws)
		if err := os.Mkdir(nestedDir, 0o700); err != nil {
			t.Fatalf("failed to create nested dir: %v", err)
		}

		nestedFile := fmt.Sprintf("%s/nested/file.txt", ws)
		if err := os.WriteFile(nestedFile, []byte("nested content"), 0o600); err != nil {
			t.Fatalf("failed to write nested file: %v", err)
		}

		// Cleanup should remove everything.
		cleanupWorkspace(ws)

		// Verify workspace and all content is gone.
		if _, err := os.Stat(ws); err == nil {
			t.Errorf("workspace %q should not exist after cleanup", ws)
		}
	})
}

// createEphemeralWorkspace creates a temporary workspace directory with a unique prefix.
func createEphemeralWorkspace() (string, error) {
	return os.MkdirTemp("", "ploy-run-*")
}

// cleanupWorkspace removes a workspace directory and all its contents.
func cleanupWorkspace(path string) {
	_ = os.RemoveAll(path)
}
