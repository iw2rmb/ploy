package nodeagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestRehydrateWorkspaceForStep_ReusesStickyWorkspaceWhenGitDirExists(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	req := StartRunRequest{
		RunID:  types.RunID("run_sticky_reuse"),
		RepoID: types.MigRepoID("repo_sticky_reuse"),
		JobID:  types.JobID("job_sticky_reuse"),
	}
	workspace := runRepoWorkspaceDir(req.RunID, req.RepoID)
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir sticky .git dir: %v", err)
	}

	rc := &runController{cfg: Config{}}
	got, err := rc.rehydrateWorkspaceForStep(context.Background(), req, contracts.StepManifest{})
	if err != nil {
		t.Fatalf("rehydrateWorkspaceForStep() error = %v, want nil", err)
	}
	if got != workspace {
		t.Fatalf("rehydrateWorkspaceForStep() path = %q, want %q", got, workspace)
	}
}

func TestRehydrateWorkspaceForStep_RemovesInvalidStickyWorkspaceBeforeRebuild(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	req := StartRunRequest{
		RunID:  types.RunID("run_sticky_invalid"),
		RepoID: types.MigRepoID("repo_sticky_invalid"),
		JobID:  types.JobID("job_sticky_invalid"),
	}
	workspace := runRepoWorkspaceDir(req.RunID, req.RepoID)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir invalid sticky workspace: %v", err)
	}
	invalidMarker := filepath.Join(workspace, "invalid.txt")
	if err := os.WriteFile(invalidMarker, []byte("invalid"), 0o644); err != nil {
		t.Fatalf("write invalid marker: %v", err)
	}

	rc := &runController{cfg: Config{}}
	if _, err := rc.rehydrateWorkspaceForStep(context.Background(), req, contracts.StepManifest{}); err == nil {
		t.Fatal("rehydrateWorkspaceForStep() error = nil, want non-nil due to missing repo hydration input")
	}
	if _, err := os.Stat(invalidMarker); !os.IsNotExist(err) {
		t.Fatalf("invalid sticky marker should be removed before rebuild, stat err = %v", err)
	}
}
