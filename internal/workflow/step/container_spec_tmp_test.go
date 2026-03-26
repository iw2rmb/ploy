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

func TestBuildContainerSpec_TmpBundleEntriesMountedReadOnly(t *testing.T) {
	stagingDir := t.TempDir()
	// Simulate extracted bundle contents in the staging dir.
	if err := os.WriteFile(filepath.Join(stagingDir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(stagingDir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}

	manifest := baseManifestForTmp(t)
	manifest.TmpBundle = &contracts.TmpBundleRef{
		BundleID: "bun-123",
		CID:      "bafy123",
		Digest:   "abc",
		Entries:  []string{"config.json", "scripts"},
	}

	spec, err := buildContainerSpec(types.RunID("run-bundle"), types.JobID("job-bundle"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// Expect workspace + 2 read-only bundle mounts.
	if len(spec.Mounts) != 3 {
		t.Fatalf("got %d mounts, want 3: %+v", len(spec.Mounts), spec.Mounts)
	}

	type want struct {
		target   string
		source   string
		readOnly bool
	}
	cases := []want{
		{target: "/tmp/config.json", source: filepath.Join(stagingDir, "config.json"), readOnly: true},
		{target: "/tmp/scripts", source: filepath.Join(stagingDir, "scripts"), readOnly: true},
	}
	for _, w := range cases {
		var found bool
		for _, m := range spec.Mounts {
			if m.Target == w.target {
				found = true
				if m.Source != w.source {
					t.Errorf("mount %s: source got %q, want %q", w.target, m.Source, w.source)
				}
				if m.ReadOnly != w.readOnly {
					t.Errorf("mount %s: ReadOnly got %v, want %v", w.target, m.ReadOnly, w.readOnly)
				}
			}
		}
		if !found {
			t.Errorf("mount %s not found in %+v", w.target, spec.Mounts)
		}
	}
}

func TestBuildContainerSpec_TmpBundleEntriesSkippedWithoutStagingDir(t *testing.T) {
	manifest := baseManifestForTmp(t)
	manifest.TmpBundle = &contracts.TmpBundleRef{
		BundleID: "bun-123",
		CID:      "bafy123",
		Digest:   "abc",
		Entries:  []string{"config.json"},
	}

	spec, err := buildContainerSpec(types.RunID("run-bundle-nostaging"), types.JobID("job-bundle-nostaging"), manifest, "/ws", "", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	for _, m := range spec.Mounts {
		if m.Target == "/tmp/config.json" {
			t.Fatalf("unexpected /tmp/config.json mount when tmpStagingDir is empty")
		}
	}
}

func TestBuildContainerSpec_TmpBundleEntriesDuplicateRejected(t *testing.T) {
	stagingDir := t.TempDir()

	manifest := baseManifestForTmp(t)
	manifest.TmpBundle = &contracts.TmpBundleRef{
		BundleID: "bun-dup",
		CID:      "bafy123",
		Digest:   "abc",
		Entries:  []string{"config.json", "config.json"},
	}

	_, err := buildContainerSpec(types.RunID("run-bundle-dup"), types.JobID("job-bundle-dup"), manifest, "/ws", "", "", stagingDir)
	if err == nil {
		t.Fatal("expected error for duplicate bundle entry, got nil")
	}
}

func TestBuildContainerSpec_TmpFilesEmptyManifestTmpDir(t *testing.T) {
	stagingDir := t.TempDir()

	manifest := baseManifestForTmp(t)
	// No bundle entries.

	spec, err := buildContainerSpec(types.RunID("run-tmp-empty"), types.JobID("job-tmp-empty"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// Only workspace mount.
	if len(spec.Mounts) != 1 {
		t.Fatalf("got %d mounts, want 1: %+v", len(spec.Mounts), spec.Mounts)
	}
}
