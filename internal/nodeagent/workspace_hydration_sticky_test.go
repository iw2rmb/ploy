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
)

func TestPrepareStickyWorkspaceForStep_ReusesStickyWorkspaceWhenGitDirExists(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	req := StartRunRequest{
		RunID:  types.RunID("run_sticky_reuse"),
		RepoID: types.RepoID("repo_sticky_reuse"),
		JobID:  types.JobID("job_sticky_reuse"),
	}
	workspace := workspaceDir(req.RunID)
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir sticky .git dir: %v", err)
	}

	rc := &runController{cfg: Config{}}
	got, err := rc.prepareStickyWorkspace(context.Background(), req)
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
		RepoID:    types.RepoID("repo_sticky_invalid"),
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
	if _, err := rc.prepareStickyWorkspace(context.Background(), req); err == nil {
		t.Fatal("prepareStickyWorkspace() error = nil, want non-nil due to missing commit SHA")
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
		RepoID:  types.RepoID("repo_sticky_missing"),
		JobID:   types.JobID("job_sticky_missing"),
		JobType: types.JobTypeMig,
	}

	rc := &runController{cfg: Config{}}
	if _, err := rc.prepareStickyWorkspace(context.Background(), req); err == nil {
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
				RepoID:    types.RepoID("repo_sticky_hydrate"),
				JobID:     types.JobID("job_sticky_hydrate"),
				JobType:   tt.jobType,
				CommitSHA: types.CommitSHA(head),
				RepoSHAIn: types.CommitSHA(head),
			}

			srv := snapshotFixtureServer(t, repoDir)
			defer srv.Close()
			rc := &runController{
				cfg:      Config{ServerURL: srv.URL, NodeID: types.NodeID("node01")},
				uploader: &baseUploader{client: srv.Client()},
			}
			workspace, err := rc.prepareStickyWorkspace(context.Background(), req)
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

// Resuming never replaces missing work with a source snapshot, even with a valid input SHA.
func TestPrepareStickyWorkspace_ResumeRequiresRetainedWorkspace(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(500) }))
	defer server.Close()
	req := StartRunRequest{RunID: "resumed-run", JobID: "failed-job", ResumeCount: 1, RepoSHAIn: types.CommitSHA("0123456789abcdef0123456789abcdef01234567")}
	rc := newTestController(t, newAgentConfig(server.URL))
	if _, err := rc.prepareStickyWorkspace(context.Background(), req); err == nil {
		t.Fatal("missing resumed workspace accepted")
	}
	if requests != 0 {
		t.Fatal("resume requested source hydration")
	}
	workspace := workspaceDir(req.RunID)
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(workspace, "partial-edit")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := rc.prepareStickyWorkspace(context.Background(), req); err != nil || got != workspace {
		t.Fatalf("retained workspace: %s %v", got, err)
	}
	if b, err := os.ReadFile(marker); err != nil || string(b) != "keep" {
		t.Fatal("resume discarded partial changes")
	}
}

// Failed extraction and commit verification both remove the unusable workspace.
func TestPrepareStickyWorkspaceRemovesFailedSnapshot(t *testing.T) {
	for _, failure := range []string{"archive path escape", "commit mismatch"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("PLOYD_CACHE_HOME", t.TempDir())
			req := newStartRunRequest()
			req.CommitSHA = "0123456789abcdef0123456789abcdef01234567"
			req.RepoSHAIn = req.CommitSHA
			var server *httptest.Server
			if failure == "commit mismatch" {
				server = snapshotFixtureServer(t, gitrepo.SetupBasic(t))
			} else {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/v1/runs/"+req.RunID.String()+"/snapshot" || r.Header.Get("PLOY_NODE_UUID") != "node01" {
						t.Errorf("snapshot request has wrong route or node authentication: %s", r.URL.Path)
					}
					gz := gzip.NewWriter(w)
					tw := tar.NewWriter(gz)
					for _, name := range []string{"partial-file", "../escape"} {
						if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: 1}); err != nil {
							t.Error(err)
						}
						_, _ = tw.Write([]byte("x"))
					}
					_ = tw.Close()
					_ = gz.Close()
				}))
			}
			defer server.Close()
			controller := &runController{
				cfg:      Config{ServerURL: server.URL, NodeID: "node01"},
				uploader: &baseUploader{client: server.Client()},
			}
			if _, err := controller.prepareStickyWorkspace(context.Background(), req); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
			if _, err := os.Stat(workspaceDir(req.RunID)); !os.IsNotExist(err) {
				t.Fatalf("failed snapshot workspace retained: %v", err)
			}
			if _, err := os.Stat(filepath.Join(runDir(req.RunID), "escape")); !os.IsNotExist(err) {
				t.Fatalf("archive escaped workspace: %v", err)
			}
		})
	}
}
