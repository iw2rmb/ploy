package step

import (
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func baseManifestForTmp(t *testing.T) contracts.StepManifest {
	t.Helper()
	return contracts.StepManifest{
		ID:    types.StepID("step-tmp"),
		Name:  "With Tmp Files",
		Image: "alpine:3",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
	}
}

func TestBuildContainerSpec_TmpFilesAreMountedReadWrite(t *testing.T) {
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "secret.txt"), []byte("s3cr3t"), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	manifest := baseManifestForTmp(t)
	manifest.TmpDir = []contracts.TmpFilePayload{
		{Name: "config.json", Content: []byte(`{}`)},
		{Name: "secret.txt", Content: []byte("s3cr3t")},
	}

	spec, err := buildContainerSpec(types.RunID("run-tmp"), types.JobID("job-tmp"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// Expect workspace + 2 tmp mounts.
	if len(spec.Mounts) != 3 {
		t.Fatalf("got %d mounts, want 3: %+v", len(spec.Mounts), spec.Mounts)
	}

	type want struct{ target, source string }
	cases := []want{
		{target: "/tmp/config.json", source: filepath.Join(stagingDir, "config.json")},
		{target: "/tmp/secret.txt", source: filepath.Join(stagingDir, "secret.txt")},
	}
	for _, w := range cases {
		var found bool
		for _, m := range spec.Mounts {
			if m.Target == w.target {
				found = true
				if m.Source != w.source {
					t.Errorf("mount %s: source got %q, want %q", w.target, m.Source, w.source)
				}
				if m.ReadOnly {
					t.Errorf("mount %s: unexpectedly read-only", w.target)
				}
			}
		}
		if !found {
			t.Errorf("mount %s not found in %+v", w.target, spec.Mounts)
		}
	}
}

func TestBuildContainerSpec_TmpFilesSkippedWhenNoStagingDir(t *testing.T) {
	manifest := baseManifestForTmp(t)
	manifest.TmpDir = []contracts.TmpFilePayload{
		{Name: "config.json", Content: []byte(`{}`)},
	}

	spec, err := buildContainerSpec(types.RunID("run-tmp-nostaging"), types.JobID("job-tmp-nostaging"), manifest, "/ws", "", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// Only workspace mount; no /tmp mounts without staging dir.
	for _, m := range spec.Mounts {
		if m.Target == "/tmp/config.json" {
			t.Fatalf("unexpected /tmp/config.json mount when tmpStagingDir is empty")
		}
	}
}

func TestBuildContainerSpec_TmpFilesTraversalNameRejected(t *testing.T) {
	stagingDir := t.TempDir()

	tests := []struct {
		name    string
		tmpName string
	}{
		{name: "path traversal", tmpName: "../escape"},
		{name: "absolute path", tmpName: "/etc/passwd"},
		{name: "dot", tmpName: "."},
		{name: "dotdot", tmpName: ".."},
		{name: "nested path", tmpName: "sub/file.txt"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			manifest := baseManifestForTmp(t)
			manifest.TmpDir = []contracts.TmpFilePayload{
				{Name: tc.tmpName, Content: []byte("data")},
			}
			_, err := buildContainerSpec(types.RunID("run-traversal"), types.JobID("job-traversal"), manifest, "/ws", "", "", stagingDir)
			if err == nil {
				t.Fatalf("buildContainerSpec: expected error for name %q, got nil", tc.tmpName)
			}
		})
	}
}

func TestBuildContainerSpec_TmpFilesEmptyManifestTmpDir(t *testing.T) {
	stagingDir := t.TempDir()

	manifest := baseManifestForTmp(t)
	// No TmpDir entries.

	spec, err := buildContainerSpec(types.RunID("run-tmp-empty"), types.JobID("job-tmp-empty"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// Only workspace mount.
	if len(spec.Mounts) != 1 {
		t.Fatalf("got %d mounts, want 1: %+v", len(spec.Mounts), spec.Mounts)
	}
}
