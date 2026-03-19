package nodeagent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestExecute_TmpDirMaterialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []contracts.TmpFilePayload
		wantErr bool
	}{
		{
			name: "single file",
			entries: []contracts.TmpFilePayload{
				{Name: "config.json", Content: []byte(`{"k":"v"}`)},
			},
		},
		{
			name: "multiple files",
			entries: []contracts.TmpFilePayload{
				{Name: "a.txt", Content: []byte("hello")},
				{Name: "b.txt", Content: []byte("world")},
			},
		},
		{
			name:    "empty entries no-ops",
			entries: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stagingDir := t.TempDir()
			err := materializeTmpFiles(tc.entries, stagingDir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("materializeTmpFiles() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}

			for _, e := range tc.entries {
				dst := filepath.Join(stagingDir, e.Name)
				got, readErr := os.ReadFile(dst)
				if readErr != nil {
					t.Errorf("entry %q: read failed: %v", e.Name, readErr)
					continue
				}
				if string(got) != string(e.Content) {
					t.Errorf("entry %q: content got %q, want %q", e.Name, got, e.Content)
				}
				// Verify read-only permissions (0o444).
				info, statErr := os.Stat(dst)
				if statErr != nil {
					t.Errorf("entry %q: stat failed: %v", e.Name, statErr)
					continue
				}
				if info.Mode().Perm() != 0o444 {
					t.Errorf("entry %q: perm got %o, want 444", e.Name, info.Mode().Perm())
				}
			}
		})
	}
}

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
