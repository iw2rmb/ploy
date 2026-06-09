package nodeagent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
	workspace := workspaceDir(req.RunID)
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir sticky .git dir: %v", err)
	}

	rc := &runController{cfg: Config{}}
	got, err := rc.prepareStickyWorkspace(context.Background(), req, contracts.StepManifest{})
	if err != nil {
		t.Fatalf("prepareStickyWorkspace() error = %v, want nil", err)
	}
	if got != workspace {
		t.Fatalf("prepareStickyWorkspace() path = %q, want %q", got, workspace)
	}
}

func TestPrepareStickyWorkspaceForStep_RemovesInvalidChainHeadWorkspaceBeforeHydration(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	req := StartRunRequest{
		RunID:     types.RunID("run_sticky_invalid"),
		RepoID:    types.MigRepoID("repo_sticky_invalid"),
		JobID:     types.JobID("job_sticky_invalid"),
		JobType:   types.JobTypePreGate,
		RepoSHAIn: types.CommitSHA("0123456789abcdef0123456789abcdef01234567"),
	}
	workspace := workspaceDir(req.RunID)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir invalid sticky workspace: %v", err)
	}
	invalidMarker := filepath.Join(workspace, "invalid.txt")
	if err := os.WriteFile(invalidMarker, []byte("invalid"), 0o644); err != nil {
		t.Fatalf("write invalid marker: %v", err)
	}

	rc := &runController{cfg: Config{}}
	if _, err := rc.prepareStickyWorkspace(context.Background(), req, contracts.StepManifest{}); err == nil {
		t.Fatal("prepareStickyWorkspace() error = nil, want non-nil due to missing repo hydration input")
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
	if _, err := rc.prepareStickyWorkspace(context.Background(), req, contracts.StepManifest{}); err == nil {
		t.Fatal("prepareStickyWorkspace() error = nil, want missing sticky workspace error")
	}
}

func TestPrepareStickyWorkspaceForStep_ChainHeadHydratesWorkspaceWithoutRunBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found")
	}

	tests := []struct {
		name    string
		jobType types.JobType
	}{
		{name: "pre gate chain head", jobType: types.JobTypePreGate},
		{name: "mig chain head", jobType: types.JobTypeMig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheHome := t.TempDir()
			t.Setenv("PLOYD_CACHE_HOME", cacheHome)
			repoDir := gitrepo.SetupBasic(t)
			head := gitrepo.RevParse(t, repoDir, "HEAD")

			req := StartRunRequest{
				RunID:     types.RunID("run_sticky_hydrate"),
				RepoID:    types.MigRepoID("repo_sticky_hydrate"),
				JobID:     types.JobID("job_sticky_hydrate"),
				JobType:   tt.jobType,
				CommitSHA: types.CommitSHA(head),
				RepoSHAIn: types.CommitSHA(head),
			}
			manifest := contracts.StepManifest{
				Inputs: []contracts.StepInput{
					{
						Name: "workspace",
						Hydration: &contracts.StepInputHydration{
							Repo: &contracts.RepoMaterialization{
								URL:     types.RepoURL("file://" + repoDir),
								BaseRef: types.GitRef("main"),
							},
						},
					},
				},
			}

			srv := snapshotFixtureServer(t, repoDir)
			defer srv.Close()
			rc := &runController{cfg: Config{ServerURL: srv.URL, NodeID: types.NodeID("node01")}, httpClient: srv.Client()}
			workspace, err := rc.prepareStickyWorkspace(context.Background(), req, manifest)
			if err != nil {
				t.Fatalf("prepareStickyWorkspace() error = %v", err)
			}
			if workspace != workspaceDir(req.RunID) {
				t.Fatalf("workspace path = %q, want %q", workspace, workspaceDir(req.RunID))
			}
			gitrepo.AssertRepo(t, workspace)
			if _, err := os.Stat(filepath.Join(runDir(req.RunID), "base")); !os.IsNotExist(err) {
				t.Fatalf("run base dir should not be created, stat err = %v", err)
			}
		})
	}
}

func snapshotFixtureServer(t *testing.T, repoDir string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		gz := gzip.NewWriter(w)
		tw := tar.NewWriter(gz)
		err := filepath.WalkDir(repoDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == repoDir {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(repoDir, path)
			if err != nil {
				return err
			}
			link := ""
			if info.Mode()&os.ModeSymlink != 0 {
				link, err = os.Readlink(path)
				if err != nil {
					return err
				}
			}
			hdr, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}
			hdr.Name = filepath.ToSlash(rel)
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if d.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(tw, f)
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		})
		if closeErr := tw.Close(); err == nil {
			err = closeErr
		}
		if closeErr := gz.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Errorf("write snapshot fixture: %v", err)
		}
	}))
}
