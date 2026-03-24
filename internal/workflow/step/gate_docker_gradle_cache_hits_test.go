package step

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestReadGradleBuildCacheHits_DeduplicatesSortsAndRemovesFile(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	hitsPath := filepath.Join(workspace, BuildGateGradleCacheHitsHostFile)
	content := []byte("\n:compileJava\n:test\n:compileJava\n")
	if err := os.WriteFile(hitsPath, content, 0o644); err != nil {
		t.Fatalf("write hits file: %v", err)
	}

	got := readGradleBuildCacheHits(workspace)
	want := []string{":compileJava", ":test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readGradleBuildCacheHits() = %v, want %v", got, want)
	}

	if _, err := os.Stat(hitsPath); !os.IsNotExist(err) {
		t.Fatalf("hits file should be removed after read, stat err=%v", err)
	}
}

func TestDockerGateExecutor_MountsGradleCacheHitsFile(t *testing.T) {
	t.Parallel()

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)
	workspace := createGradleWorkspace(t, "17")

	spec := &contracts.StepGateSpec{Enabled: true}
	if _, err := executor.Execute(context.Background(), spec, workspace); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	wantSource := filepath.Join(workspace, BuildGateGradleCacheHitsHostFile)
	found := false
	for _, mount := range rt.captured.Mounts {
		if mount.Source == wantSource && mount.Target == BuildGateGradleCacheHitsContainerFile && !mount.ReadOnly {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf(
			"expected gradle cache hits mount %q -> %q in mounts=%+v",
			wantSource,
			BuildGateGradleCacheHitsContainerFile,
			rt.captured.Mounts,
		)
	}
}
