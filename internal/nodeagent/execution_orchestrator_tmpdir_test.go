package nodeagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
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
		{
			name:    "path traversal rejected",
			entries: []contracts.TmpFilePayload{{Name: "../escape", Content: []byte("data")}},
			wantErr: true,
		},
		{
			name:    "nested path rejected",
			entries: []contracts.TmpFilePayload{{Name: "sub/file.txt", Content: []byte("data")}},
			wantErr: true,
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

func TestExecute_TmpDirMaterialization_CanonicalizesNameWhitespace(t *testing.T) {
	t.Parallel()

	stagingDir := t.TempDir()
	entries := []contracts.TmpFilePayload{
		{Name: " config.json ", Content: []byte(`{"k":"v"}`)},
	}

	if err := materializeTmpFiles(entries, stagingDir); err != nil {
		t.Fatalf("materializeTmpFiles() unexpected error: %v", err)
	}

	canonicalPath := filepath.Join(stagingDir, "config.json")
	if _, err := os.Stat(canonicalPath); err != nil {
		t.Fatalf("expected canonical tmp file %q: %v", canonicalPath, err)
	}

	nonCanonicalPath := filepath.Join(stagingDir, " config.json ")
	if _, err := os.Stat(nonCanonicalPath); !os.IsNotExist(err) {
		t.Fatalf("expected non-canonical tmp file path %q to be absent", nonCanonicalPath)
	}
}

// TestRunRouterForGateFailure_TmpDirMaterialization verifies that when a router
// spec contains tmpDir entries, runRouterForGateFailure materializes them into a
// staging directory, mounts each file read-only at /tmp/<name>, and removes the
// staging directory after execution (both success and early-return paths).
func TestRunRouterForGateFailure_TmpDirMaterialization(t *testing.T) {
	t.Parallel()

	rc := &runController{cfg: Config{ServerURL: "http://localhost:9999"}}
	workspace := t.TempDir()

	// Capture the ContainerSpec passed to Create so we can assert mounts.
	var mu sync.Mutex
	var capturedSpec step.ContainerSpec
	var capturedStagingDir string

	mc := &mockRouterContainerRuntime{}
	mc.createFn = func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		mu.Lock()
		capturedSpec = spec
		// Capture the staging dir from the /tmp/secret.txt mount source so we can
		// verify it is removed after runRouterForGateFailure returns.
		for _, m := range spec.Mounts {
			if m.Target == "/tmp/secret.txt" {
				// Source is <stagingDir>/secret.txt; strip the filename.
				capturedStagingDir = strings.TrimSuffix(m.Source, "/secret.txt")
			}
		}
		mu.Unlock()
		// Write a minimal codex-last.txt so parseBugSummary / parseRouterDecision succeed.
		for _, m := range spec.Mounts {
			if m.Target == "/out" {
				_ = os.WriteFile(filepath.Join(m.Source, "codex-last.txt"),
					[]byte(`{"error_kind":"infra","strategy_id":"s"}`+"\n"), 0o644)
			}
		}
		return step.ContainerHandle("mock"), nil
	}

	runner := step.Runner{Containers: mc}

	req := StartRunRequest{
		RunID:   types.RunID("run-router-tmpdir"),
		JobID:   types.JobID("job-router-tmpdir"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		JobType: types.JobTypePreGate,
	}
	typedOpts := RunOptions{
		HealingSelector: &contracts.HealingSpec{
			ByErrorKind: map[string]contracts.HealingActionSpec{
				"infra": {Image: contracts.JobImage{Universal: "test/healer:latest"}},
			},
		},
		Healing: &HealingConfig{
			Retries: 1,
			Mod:     ModContainerSpec{Image: contracts.JobImage{Universal: "test/healer:latest"}},
		},
		Router: &ModContainerSpec{
			Image: contracts.JobImage{Universal: "test/router:latest"},
			TmpDir: []contracts.TmpFilePayload{
				{Name: "secret.txt", Content: []byte("s3cr3t")},
				{Name: "config.json", Content: []byte(`{"k":"v"}`)},
			},
		},
	}
	gateResult := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
		LogsText:     "[ERROR] build failed\n",
	}

	rc.runRouterForGateFailure(context.Background(), runner, req, typedOpts, workspace, gateResult)

	mu.Lock()
	spec := capturedSpec
	stagingDir := capturedStagingDir
	mu.Unlock()

	// Assert /tmp/secret.txt and /tmp/config.json mounts are present and read-only.
	wantMounts := map[string]bool{
		"/tmp/secret.txt":  false,
		"/tmp/config.json": false,
	}
	for _, m := range spec.Mounts {
		if _, ok := wantMounts[m.Target]; ok {
			wantMounts[m.Target] = true
			if !m.ReadOnly {
				t.Errorf("mount %q: want ReadOnly=true, got false", m.Target)
			}
		}
	}
	for target, found := range wantMounts {
		if !found {
			t.Errorf("expected mount at %q was not present in ContainerSpec", target)
		}
	}

	// Assert the staging directory is cleaned up after runRouterForGateFailure returns.
	if stagingDir == "" {
		t.Fatal("capturedStagingDir is empty; /tmp/secret.txt mount not found in ContainerSpec")
	}
	if _, statErr := os.Stat(stagingDir); !os.IsNotExist(statErr) {
		t.Fatalf("router tmp staging dir %q still exists after runRouterForGateFailure returned; want removed", stagingDir)
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
