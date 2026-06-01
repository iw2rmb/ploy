package nodeagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRunRepoArtifactPaths(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	jobID := types.NewJobID()

	wantRunRoot := filepath.Join(cacheHome, "runs", runID.String())
	if got := runCacheDir(runID); got != wantRunRoot {
		t.Fatalf("runCacheDir() = %q, want %q", got, wantRunRoot)
	}
	if strings.Contains(runCacheDir(runID), filepath.Join("ploy", "run")) {
		t.Fatalf("run cache path uses old ploy/run layout: %s", runCacheDir(runID))
	}

	wantRepoRoot := wantRunRoot
	if got := runRepoRootDir(runID, repoID); got != wantRepoRoot {
		t.Fatalf("runRepoRootDir() = %q, want %q", got, wantRepoRoot)
	}
	if got := runRepoWorkspaceDir(runID, repoID); got != filepath.Join(wantRepoRoot, "workspace") {
		t.Fatalf("runRepoWorkspaceDir() = %q", got)
	}
	if got := runRepoArtifactsDir(runID, repoID); got != filepath.Join(wantRepoRoot, "artifacts") {
		t.Fatalf("runRepoArtifactsDir() = %q", got)
	}
	if got := runRepoSharedArtifactsDir(runID, repoID); got != filepath.Join(wantRepoRoot, "artifacts", "shared") {
		t.Fatalf("runRepoSharedArtifactsDir() = %q", got)
	}

	paths := runRepoJobArtifactPaths(runID, repoID, jobID)
	wantJobRoot := filepath.Join(wantRepoRoot, "artifacts", jobID.String())
	if paths.Root != wantJobRoot {
		t.Fatalf("job artifact root = %q, want %q", paths.Root, wantJobRoot)
	}
	if paths.In != filepath.Join(wantJobRoot, "in") || paths.Out != filepath.Join(wantJobRoot, "out") {
		t.Fatalf("job in/out paths = %q/%q", paths.In, paths.Out)
	}
	if paths.Stdout != filepath.Join(wantJobRoot, "stdout.log") || paths.Stderr != filepath.Join(wantJobRoot, "stderr.log") || paths.Diff != filepath.Join(wantJobRoot, "diff.patch") {
		t.Fatalf("job log/diff paths = %+v", paths)
	}

	repoID = types.MigRepoID("")
	if got := runRepoRootDir(runID, repoID); got != "" {
		t.Fatalf("runRepoRootDir zero repo = %q, want empty", got)
	}
	if got := runRepoWorkspaceDir(runID, repoID); got != "" {
		t.Fatalf("runRepoWorkspaceDir zero repo = %q, want empty", got)
	}
	if got := runRepoJobArtifactPaths(runID, repoID, jobID); got != (jobArtifactPaths{}) {
		t.Fatalf("job artifact paths zero repo = %+v, want empty", got)
	}
}

func TestEnsureJobArtifactDirs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts", "job")
	paths := jobArtifactPaths{
		Root: root,
		In:   filepath.Join(root, "in"),
		Out:  filepath.Join(root, "out"),
	}
	if err := ensureJobArtifactDirs(paths); err != nil {
		t.Fatalf("ensureJobArtifactDirs() error = %v", err)
	}
	for _, dir := range []string{paths.In, paths.Out} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected dir %s, info=%v err=%v", dir, info, err)
		}
	}
}
