package nodeagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestPrepareStickyWorkspaceForStep_ReusesStickyWorkspaceWhenGitDirExists(t *testing.T) {
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
	got, err := rc.prepareStickyWorkspaceForStep(context.Background(), req, contracts.StepManifest{})
	if err != nil {
		t.Fatalf("prepareStickyWorkspaceForStep() error = %v, want nil", err)
	}
	if got != workspace {
		t.Fatalf("prepareStickyWorkspaceForStep() path = %q, want %q", got, workspace)
	}
}

func TestPrepareStickyWorkspaceForStep_RemovesInvalidChainHeadWorkspaceBeforeHydration(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	req := StartRunRequest{
		RunID:   types.RunID("run_sticky_invalid"),
		RepoID:  types.MigRepoID("repo_sticky_invalid"),
		JobID:   types.JobID("job_sticky_invalid"),
		JobType: types.JobTypePreGate,
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
	if _, err := rc.prepareStickyWorkspaceForStep(context.Background(), req, contracts.StepManifest{}); err == nil {
		t.Fatal("prepareStickyWorkspaceForStep() error = nil, want non-nil due to missing repo hydration input")
	}
	if _, err := os.Stat(invalidMarker); !os.IsNotExist(err) {
		t.Fatalf("invalid sticky marker should be removed before rebuild, stat err = %v", err)
	}
}

func TestPrepareStickyWorkspaceForStep_MissingNonHeadWorkspaceFails(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	req := StartRunRequest{
		RunID:   types.RunID("run_sticky_missing"),
		RepoID:  types.MigRepoID("repo_sticky_missing"),
		JobID:   types.JobID("job_sticky_missing"),
		JobType: types.JobTypeMig,
	}

	rc := &runController{cfg: Config{}}
	if _, err := rc.prepareStickyWorkspaceForStep(context.Background(), req, contracts.StepManifest{}); err == nil {
		t.Fatal("prepareStickyWorkspaceForStep() error = nil, want missing sticky workspace error")
	}
}

func TestPrepareStickyWorkspaceForStep_ChainHeadHydratesWorkspaceWithoutRunBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found")
	}

	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)
	repoDir := gitrepo.SetupBasic(t)

	req := StartRunRequest{
		RunID:   types.RunID("run_sticky_hydrate"),
		RepoID:  types.MigRepoID("repo_sticky_hydrate"),
		JobID:   types.JobID("job_sticky_hydrate"),
		JobType: types.JobTypePreGate,
	}
	manifest := contracts.StepManifest{
		Inputs: []contracts.StepInput{
			{
				Name: "workspace",
				Hydration: &contracts.StepInputHydration{
					Repo: &contracts.RepoMaterialization{
						URL:       types.RepoURL("file://" + repoDir),
						BaseRef:   types.GitRef("main"),
						TargetRef: types.GitRef("main"),
					},
				},
			},
		},
	}

	rc := &runController{cfg: Config{}}
	workspace, err := rc.prepareStickyWorkspaceForStep(context.Background(), req, manifest)
	if err != nil {
		t.Fatalf("prepareStickyWorkspaceForStep() error = %v", err)
	}
	if workspace != runRepoWorkspaceDir(req.RunID, req.RepoID) {
		t.Fatalf("workspace path = %q, want %q", workspace, runRepoWorkspaceDir(req.RunID, req.RepoID))
	}
	gitrepo.AssertRepo(t, workspace)
	if _, err := os.Stat(filepath.Join(runCacheDir(req.RunID), "base")); !os.IsNotExist(err) {
		t.Fatalf("run base dir should not be created, stat err = %v", err)
	}
}
