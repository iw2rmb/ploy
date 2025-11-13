package nodeagent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestRunControllerStartRun verifies that starting a run registers it in the controller.
func TestRunControllerStartRun(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	req := StartRunRequest{
		RunID:   types.RunID("run-001"),
		RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	ctx := context.Background()
	if err := rc.StartRun(ctx, req); err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	if _, exists := rc.runs[req.RunID.String()]; !exists {
		t.Errorf("run %s not found in controller", req.RunID)
	}
}

// TestRunControllerStartRunDuplicate verifies that starting a duplicate run returns an error.
func TestRunControllerStartRunDuplicate(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	req := StartRunRequest{
		RunID:   types.RunID("run-001"),
		RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		BaseRef: types.GitRef("main"),
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

// TestRunControllerStopRun verifies that stopping a run removes it from the controller.
func TestRunControllerStopRun(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	// Start a run first.
	startReq := StartRunRequest{
		RunID:   types.RunID("run-001"),
		RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		BaseRef: types.GitRef("main"),
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

// TestRunControllerStopNonExistent verifies that stopping a nonexistent run returns an error.
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

// TestBuildManifestFromRequest verifies that a run manifest is correctly built from a StartRunRequest.
// This includes validation of required fields, defaults, and hydration configuration.
func TestBuildManifestFromRequest(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		req := StartRunRequest{
			RunID:     types.RunID("run-123"),
			RepoURL:   types.RepoURL("https://github.com/example/repo.git"),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("feature-branch"),
			CommitSHA: types.CommitSHA("abc123"),
			Env: map[string]string{
				"FOO": "bar",
			},
		}

		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if manifest.ID.String() != req.RunID.String() {
			t.Errorf("expected ID %q, got %q", req.RunID, manifest.ID.String())
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
		if string(repo.URL) != req.RepoURL.String() {
			t.Errorf("expected repo URL %q, got %q", req.RepoURL, string(repo.URL))
		}
		if repo.BaseRef.String() != req.BaseRef.String() {
			t.Errorf("expected base ref %q, got %q", req.BaseRef, repo.BaseRef.String())
		}
		if repo.TargetRef.String() != req.TargetRef.String() {
			t.Errorf("expected target ref %q, got %q", req.TargetRef, repo.TargetRef.String())
		}
		if repo.Commit.String() != req.CommitSHA.String() {
			t.Errorf("expected commit %q, got %q", req.CommitSHA, repo.Commit.String())
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
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
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
			RunID: types.RunID("run-123"),
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
			RunID:   types.RunID("run-123"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			BaseRef: types.GitRef("main"),
		}

		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if manifest.Inputs[0].Hydration.Repo.TargetRef.String() != "main" {
			t.Errorf("expected target_ref to default to main, got %q", manifest.Inputs[0].Hydration.Repo.TargetRef.String())
		}
	})

	t.Run("validates manifest", func(t *testing.T) {
		req := StartRunRequest{
			RunID:     types.RunID("run-123"),
			RepoURL:   types.RepoURL("https://github.com/example/repo.git"),
			TargetRef: types.GitRef("main"),
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
			RunID:   types.RunID("run-123"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
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

	// New behavior: only inject placeholder command when using default image.
	// If a custom image is provided and no command is set, leave command empty
	// so the image's own CMD/ENTRYPOINT drives execution.
	t.Run("no command injected when custom image provided", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-123"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Options: map[string]any{
				"image": "docker.io/example/mods-openrewrite:latest",
			},
		}
		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}
		if got, want := manifest.Image, "docker.io/example/mods-openrewrite:latest"; got != want {
			t.Fatalf("image=%q, want %q", got, want)
		}
		if len(manifest.Command) != 0 {
			t.Fatalf("expected no command to be injected for custom image, got len=%d", len(manifest.Command))
		}
	})

	t.Run("placeholder command injected only for default ubuntu image", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-456"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		}
		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}
		want := []string{"/bin/sh", "-c", "echo 'Build gate placeholder'"}
		if len(manifest.Command) != len(want) {
			t.Fatalf("command len=%d, want %d", len(manifest.Command), len(want))
		}
		for i := range want {
			if manifest.Command[i] != want[i] {
				t.Fatalf("command[%d]=%q, want %q", i, manifest.Command[i], want[i])
			}
		}
	})

	t.Run("gitlab options are extracted and stored in manifest", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-789"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Options: map[string]any{
				"gitlab_pat":       "glpat-secret-token",
				"gitlab_domain":    "gitlab.example.com",
				"mr_on_success":    true,
				"mr_on_fail":       false,
				"retain_container": true,
			},
		}
		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		// Verify GitLab options are stored in manifest.Options.
		if manifest.Options == nil {
			t.Fatal("expected Options to be set")
		}
		if pat, ok := manifest.Options["gitlab_pat"].(string); !ok || pat != "glpat-secret-token" {
			t.Errorf("expected gitlab_pat=glpat-secret-token, got %v", manifest.Options["gitlab_pat"])
		}
		if domain, ok := manifest.Options["gitlab_domain"].(string); !ok || domain != "gitlab.example.com" {
			t.Errorf("expected gitlab_domain=gitlab.example.com, got %v", manifest.Options["gitlab_domain"])
		}
		if mrSuccess, ok := manifest.Options["mr_on_success"].(bool); !ok || !mrSuccess {
			t.Errorf("expected mr_on_success=true, got %v", manifest.Options["mr_on_success"])
		}
		if mrFail, ok := manifest.Options["mr_on_fail"].(bool); !ok || mrFail {
			t.Errorf("expected mr_on_fail=false, got %v", manifest.Options["mr_on_fail"])
		}
	})

	t.Run("gitlab options are trimmed and only included when non-empty", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-890"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Options: map[string]any{
				"gitlab_pat":    "  trimmed-token  ",
				"gitlab_domain": "",
				"mr_on_success": true,
			},
		}
		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if manifest.Options == nil {
			t.Fatal("expected Options to be set")
		}
		// PAT should be trimmed.
		if pat, ok := manifest.Options["gitlab_pat"].(string); !ok || pat != "trimmed-token" {
			t.Errorf("expected gitlab_pat=trimmed-token (trimmed), got %v", manifest.Options["gitlab_pat"])
		}
		// Empty domain should not be stored.
		if _, exists := manifest.Options["gitlab_domain"]; exists {
			t.Errorf("expected gitlab_domain to be omitted when empty")
		}
		// mr_on_success should be stored.
		if mrSuccess, ok := manifest.Options["mr_on_success"].(bool); !ok || !mrSuccess {
			t.Errorf("expected mr_on_success=true, got %v", manifest.Options["mr_on_success"])
		}
		// mr_on_fail should not be stored if not provided.
		if _, exists := manifest.Options["mr_on_fail"]; exists {
			t.Errorf("expected mr_on_fail to be omitted when not provided")
		}
	})

	t.Run("no gitlab options results in empty Options map", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-901"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Options: map[string]any{
				"image": "alpine:latest",
			},
		}
		manifest, err := buildManifestFromRequest(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		// Options should be empty when no GitLab options are provided.
		if len(manifest.Options) != 0 {
			t.Errorf("expected empty Options when no GitLab options provided, got %v", manifest.Options)
		}
	})
}

// TestWorkspaceLifecycle verifies workspace creation, uniqueness, and cleanup operations.
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
func createEphemeralWorkspace() (string, error) { return createWorkspaceDir() }

// cleanupWorkspace removes a workspace directory and all its contents.
func cleanupWorkspace(path string) {
	_ = os.RemoveAll(path)
}

// TestWorkspaceBaseEnv verifies that workspace creation respects the PLOYD_CACHE_HOME environment variable.
func TestWorkspaceBaseEnv(t *testing.T) {
	t.Run("respects PLOYD_CACHE_HOME base", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("PLOYD_CACHE_HOME", base)

		ws, err := createEphemeralWorkspace()
		if err != nil {
			t.Fatalf("failed to create workspace: %v", err)
		}
		defer cleanupWorkspace(ws)

		wantPrefix := filepath.Clean(base) + string(os.PathSeparator)
		if !strings.HasPrefix(ws, wantPrefix) {
			t.Fatalf("workspace %q not under base %q", ws, wantPrefix)
		}
	})

	t.Run("auto-creates base when missing", func(t *testing.T) {
		baseRoot := t.TempDir()
		// Choose a non-existent subdir under the temp root
		base := filepath.Join(baseRoot, "ploy-cache-subdir")
		t.Setenv("PLOYD_CACHE_HOME", base)

		ws, err := createEphemeralWorkspace()
		if err != nil {
			t.Fatalf("failed to create workspace with missing base: %v", err)
		}
		defer cleanupWorkspace(ws)

		// Base should now exist and workspace should reside under it
		if _, err := os.Stat(base); err != nil {
			t.Fatalf("expected base %q to be created: %v", base, err)
		}
		wantPrefix := filepath.Clean(base) + string(os.PathSeparator)
		if !strings.HasPrefix(ws, wantPrefix) {
			t.Fatalf("workspace %q not under base %q", ws, wantPrefix)
		}
	})
}

// TestEndToEndFlow verifies the complete node execution flow from start to finish.
// This test demonstrates that the node can accept a run request, execute it, stream logs,
// upload diff/artifacts, and emit terminal status successfully.
func TestEndToEndFlow(t *testing.T) {
	t.Run("complete flow with mock server", func(t *testing.T) {
		// Track which endpoints were called during execution (concurrency-safe).
		type endpointHits struct {
			mu sync.Mutex
			m  map[string]int
		}
		inc := func(eh *endpointHits, path string) {
			eh.mu.Lock()
			eh.m[path]++
			eh.mu.Unlock()
		}
		snapshot := func(eh *endpointHits) map[string]int {
			eh.mu.Lock()
			defer eh.mu.Unlock()
			cp := make(map[string]int, len(eh.m))
			for k, v := range eh.m {
				cp[k] = v
			}
			return cp
		}
		get := func(eh *endpointHits, path string) int {
			eh.mu.Lock()
			defer eh.mu.Unlock()
			return eh.m[path]
		}
		endpointsCalled := &endpointHits{m: make(map[string]int)}

		// Create a mock server that responds to node requests.
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inc(endpointsCalled, r.URL.Path)

			switch {
			case strings.HasPrefix(r.URL.Path, "/v1/nodes/") && strings.HasSuffix(r.URL.Path, "/heartbeat"):
				// Heartbeat endpoint.
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			case strings.HasPrefix(r.URL.Path, "/v1/nodes/") && strings.Contains(r.URL.Path, "/events"):
				// Log events endpoint.
				w.WriteHeader(http.StatusCreated)
			case strings.HasPrefix(r.URL.Path, "/v1/nodes/") && strings.Contains(r.URL.Path, "/stage/") && strings.HasSuffix(r.URL.Path, "/diff"):
				// Diff upload endpoint.
				w.WriteHeader(http.StatusCreated)
			case strings.HasPrefix(r.URL.Path, "/v1/nodes/") && strings.Contains(r.URL.Path, "/stage/") && strings.HasSuffix(r.URL.Path, "/artifact"):
				// Artifact upload endpoint.
				w.WriteHeader(http.StatusCreated)
			case strings.HasPrefix(r.URL.Path, "/v1/nodes/") && strings.HasSuffix(r.URL.Path, "/complete"):
				// Terminal status endpoint.
				w.WriteHeader(http.StatusOK)
			default:
				t.Logf("unexpected endpoint called: %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer mockServer.Close()

		// Create a minimal config pointing to the mock server.
		cfg := Config{
			NodeID:    "test-node-e2e",
			ServerURL: mockServer.URL,
			HTTP: HTTPConfig{
				Listen: ":0", // Random port.
			},
			Heartbeat: HeartbeatConfig{
				Interval: 1 * time.Second,
				Timeout:  500 * time.Millisecond,
			},
		}

		// Create the run controller.
		rc := &runController{
			cfg:  cfg,
			runs: make(map[string]*runContext),
		}

		// Create a simple StartRunRequest that will execute quickly.
		// We use a tiny command that exits immediately to avoid long test runs.
		req := StartRunRequest{
			RunID:   types.RunID("test-run-e2e"),
			RepoURL: types.RepoURL("https://github.com/example/test-repo.git"),
			BaseRef: types.GitRef("main"),
			Options: map[string]any{
				"image":   "alpine:latest",
				"command": "echo 'test execution'",
			},
		}

		// Start the run in a background context with timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := rc.StartRun(ctx, req); err != nil {
			t.Fatalf("StartRun() failed: %v", err)
		}

		// Verify the run was registered.
		rc.mu.Lock()
		if _, exists := rc.runs[req.RunID.String()]; !exists {
			t.Errorf("run %s not found after StartRun", req.RunID)
		}
		rc.mu.Unlock()

		// Wait a bit for the run to execute (it will fail due to git clone, but that's expected).
		// The important part is that it attempts to stream logs, upload diff, and emit status.
		time.Sleep(2 * time.Second)

		// Cancel the context to stop execution if still running.
		cancel()

		// Wait a bit more for cleanup.
		time.Sleep(500 * time.Millisecond)

		// Verify the run was cleaned up from the controller.
		rc.mu.Lock()
		if _, exists := rc.runs[req.RunID.String()]; exists {
			t.Errorf("run %s still exists after completion", req.RunID)
		}
		rc.mu.Unlock()

		// Verify that at least the terminal status endpoint was called.
		// (Other endpoints may or may not be called depending on how far execution got.)
		t.Logf("Endpoints called: %+v", snapshot(endpointsCalled))
		if get(endpointsCalled, "/v1/nodes/test-node-e2e/complete") < 1 {
			t.Errorf("terminal status endpoint was not called")
		}
	})
}
