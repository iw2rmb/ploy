package nodeagent

import (
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestWithTempDir(t *testing.T) {
	tests := []struct {
		name      string
		fnErr     error
		wantErr   error
	}{
		{name: "creates_and_cleans_up", fnErr: nil, wantErr: nil},
		{name: "cleans_up_on_error", fnErr: os.ErrInvalid, wantErr: os.ErrInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedDir string
			err := withTempDir("test-*", func(dir string) error {
				capturedDir = dir
				if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
					t.Fatalf("temp directory does not exist: %s", dir)
				}
				if tt.fnErr == nil {
					testFile := filepath.Join(dir, "test.txt")
					if writeErr := os.WriteFile(testFile, []byte("test"), 0o644); writeErr != nil {
						t.Fatalf("failed to create test file: %v", writeErr)
					}
				}
				return tt.fnErr
			})
			if err != tt.wantErr {
				t.Fatalf("withTempDir error = %v, want %v", err, tt.wantErr)
			}
			if _, statErr := os.Stat(capturedDir); !os.IsNotExist(statErr) {
				t.Fatalf("temp directory was not cleaned up: %s", capturedDir)
			}
		})
	}
}

func TestSnapshotWorkspaceForNoIndexDiff(t *testing.T) {
	runID := types.RunID("test-run")
	jobID := types.JobID("test-job")

	tests := []struct {
		name      string
		setupGit  bool
		addFile   bool
		wantEmpty bool
	}{
		{name: "creates_snapshot", setupGit: true, addFile: true},
		{name: "cleanup_works", setupGit: true},
		{name: "no_git_repo_returns_empty", wantEmpty: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			if tt.setupGit {
				if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
					t.Fatalf("create .git: %v", err)
				}
			}
			if tt.addFile {
				if err := os.WriteFile(filepath.Join(workspace, "test.txt"), []byte("content"), 0o644); err != nil {
					t.Fatalf("create test file: %v", err)
				}
			}

			result := snapshotWorkspaceForNoIndexDiff(runID, jobID, types.DiffJobTypeMod, workspace)
			defer result.cleanup()

			if tt.wantEmpty {
				if result.path != "" {
					t.Fatalf("expected empty dir for non-git workspace, got: %s", result.path)
				}
				return
			}

			if result.path == "" {
				t.Fatal("snapshot directory is empty")
			}

			if tt.addFile {
				data, err := os.ReadFile(filepath.Join(result.path, "test.txt"))
				if err != nil {
					t.Fatalf("read copied file: %v", err)
				}
				if string(data) != "content" {
					t.Fatalf("copied file content = %q, want %q", data, "content")
				}
			}

			// Verify cleanup removes the directory.
			snapshotDir := result.path
			result.cleanup()
			if _, err := os.Stat(snapshotDir); !os.IsNotExist(err) {
				t.Fatalf("snapshot directory was not cleaned up: %s", snapshotDir)
			}
		})
	}
}

func TestClearManifestHydration(t *testing.T) {
	tests := []struct {
		name   string
		inputs []contracts.StepInput
	}{
		{
			name: "removes_hydration",
			inputs: []contracts.StepInput{
				{Name: "input1", Hydration: &contracts.StepInputHydration{}},
				{Name: "input2", Hydration: &contracts.StepInputHydration{}},
			},
		},
		{
			name:   "empty_inputs_noop",
			inputs: []contracts.StepInput{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := contracts.StepManifest{Inputs: tt.inputs}
			clearManifestHydration(&manifest)
			for i, input := range manifest.Inputs {
				if input.Hydration != nil {
					t.Errorf("input[%d].Hydration should be nil", i)
				}
			}
		})
	}
}

func TestDisableManifestGate(t *testing.T) {
	tests := []struct {
		name string
		gate *contracts.StepGateSpec
	}{
		{name: "sets_gate_disabled", gate: &contracts.StepGateSpec{Enabled: true}},
		{name: "nil_gate_sets_disabled", gate: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := contracts.StepManifest{Gate: tt.gate}
			disableManifestGate(&manifest)
			if manifest.Gate == nil {
				t.Fatal("Gate should not be nil")
			}
			if manifest.Gate.Enabled {
				t.Error("Gate.Enabled should be false")
			}
		})
	}
}

func TestWorkspaceRehydrationResult_CleanupRemovesDirectory(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "test.txt"), []byte("test"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	result := tempResource{
		path:    workspace,
		cleanup: func() { _ = os.RemoveAll(workspace) },
	}
	result.cleanup()

	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("workspace was not cleaned up: %s", workspace)
	}
}

func TestSnapshotResult_CleanupIsSafeWhenEmpty(t *testing.T) {
	result := tempResource{
		path:    "",
		cleanup: func() {},
	}
	result.cleanup()
}
