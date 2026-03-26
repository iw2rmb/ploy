package nodeagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanup_TmpStagingDir(t *testing.T) {
	t.Parallel()

	// Verify that withTempDir removes the staging dir on return (success path).
	var capturedDir string
	err := withTempDir("ploy-tmpfiles-test-*", func(dir string) error {
		capturedDir = dir
		dst := filepath.Join(dir, "file.txt")
		return os.WriteFile(dst, []byte("data"), 0o444)
	})
	if err != nil {
		t.Fatalf("withTempDir error: %v", err)
	}
	if _, statErr := os.Stat(capturedDir); !os.IsNotExist(statErr) {
		t.Fatalf("staging dir %q still exists after withTempDir returned; want removed", capturedDir)
	}
}

func TestCleanup_TmpStagingDir_OnError(t *testing.T) {
	t.Parallel()

	// Verify that withTempDir removes the staging dir even when fn returns an error.
	var capturedDir string
	_ = withTempDir("ploy-tmpfiles-test-err-*", func(dir string) error {
		capturedDir = dir
		return os.ErrInvalid
	})
	if _, statErr := os.Stat(capturedDir); !os.IsNotExist(statErr) {
		t.Fatalf("staging dir %q still exists after withTempDir error return; want removed", capturedDir)
	}
}
