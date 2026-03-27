package nodeagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Workspace helpers and env tests: ephemeral dir creation, cleanup, and base env.

// createEphemeralWorkspace creates a temporary workspace directory with a unique prefix.
func createEphemeralWorkspace() (string, error) { return createWorkspaceDir() }

// cleanupWorkspace removes a workspace directory and all its contents.
func cleanupWorkspace(path string) {
	_ = os.RemoveAll(path)
}

func TestWorkspaceHelpers(t *testing.T) {
	t.Run("workspace creation produces unique directory", func(t *testing.T) {
		ws, err := createEphemeralWorkspace()
		if err != nil {
			t.Fatalf("expected no error creating workspace, got %v", err)
		}
		defer cleanupWorkspace(ws)

		if _, err := os.Stat(ws); err != nil {
			t.Fatalf("expected workspace to exist at %q, got error: %v", ws, err)
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
