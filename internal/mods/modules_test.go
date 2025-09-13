package transflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"os"

	"github.com/iw2rmb/ploy/internal/cli/common"
	provider "github.com/iw2rmb/ploy/internal/git/provider"
)

// BuildGateAdapter should delegate to BuildCheckerInterface
func TestBuildGateAdapter_Delegates(t *testing.T) {
	mock := NewMockBuildChecker()
	gate := NewBuildGateAdapter(mock)
	cfg := common.DeployConfig{App: "a", Lane: "C", Environment: "dev", Timeout: time.Second}
	res, err := gate.Check(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.BuildCalled {
		t.Fatalf("expected BuildChecker called")
	}
	if mock.BuildConfig.App != "a" || mock.BuildConfig.Lane != "C" {
		t.Fatalf("unexpected cfg: %+v", mock.BuildConfig)
	}
	if res == nil || !res.Success {
		t.Fatalf("unexpected result: %+v", res)
	}
}

// RepoManagerAdapter should delegate to GitOperationsInterface
type fakeGitOps struct{ calls []string }

func (f *fakeGitOps) CloneRepository(ctx context.Context, repoURL, branch, targetPath string) error {
	f.calls = append(f.calls, "clone:"+repoURL+":"+branch+":"+targetPath)
	return nil
}
func (f *fakeGitOps) CreateBranchAndCheckout(ctx context.Context, repoPath, branchName string) error {
	f.calls = append(f.calls, "branch:"+repoPath+":"+branchName)
	return nil
}
func (f *fakeGitOps) CommitChanges(ctx context.Context, repoPath, message string) error {
	f.calls = append(f.calls, "commit:"+repoPath)
	return nil
}
func (f *fakeGitOps) PushBranch(ctx context.Context, repoPath, remoteURL, branchName string) error {
	f.calls = append(f.calls, "push:"+repoPath+":"+remoteURL+":"+branchName)
	return nil
}

func TestRepoManagerAdapter_Delegates(t *testing.T) {
	f := &fakeGitOps{}
	rm := NewRepoManagerAdapter(f)
	if err := rm.Clone(context.Background(), "u", "b", "/tmp/x"); err != nil {
		t.Fatal(err)
	}
	if err := rm.CreateBranch(context.Background(), "/tmp/x", "br"); err != nil {
		t.Fatal(err)
	}
	if err := rm.Commit(context.Background(), "/tmp/x", "m"); err != nil {
		t.Fatal(err)
	}
	if err := rm.Push(context.Background(), "/tmp/x", "origin", "br"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(f.calls), f.calls)
	}
}

// MRManagerAdapter should validate config and create MR
type fakeGitProv struct {
	validateErr error
	res         *provider.MRResult
	gotCfg      provider.MRConfig
}

func (f *fakeGitProv) CreateOrUpdateMR(ctx context.Context, cfg provider.MRConfig) (*provider.MRResult, error) {
	f.gotCfg = cfg
	return f.res, nil
}
func (f *fakeGitProv) ValidateConfiguration() error { return f.validateErr }

func TestMRManagerAdapter_Delegates(t *testing.T) {
	gp := &fakeGitProv{res: &provider.MRResult{MRURL: "https://x/mr/1", Created: true}}
	mm := NewMRManagerAdapter(gp)
	url, meta, err := mm.CreateOrUpdate(context.Background(), provider.MRConfig{RepoURL: "r", SourceBranch: "s", TargetBranch: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://x/mr/1" {
		t.Fatalf("unexpected url: %s", url)
	}
	if c, _ := meta["created"].(bool); !c {
		t.Fatalf("expected created=true")
	}
	if gp.gotCfg.RepoURL != "r" || gp.gotCfg.SourceBranch != "s" || gp.gotCfg.TargetBranch != "t" {
		t.Fatalf("bad cfg: %+v", gp.gotCfg)
	}
}

func TestMRManagerAdapter_ValidationError(t *testing.T) {
	gp := &fakeGitProv{validateErr: errors.New("bad cfg")}
	mm := NewMRManagerAdapter(gp)
	if _, _, err := mm.CreateOrUpdate(context.Background(), provider.MRConfig{}); err == nil {
		t.Fatalf("expected error")
	}
}

// TransformationExecutorAdapter should call submit helper; we'll stub validate/submit to succeed
func TestTransformationExecutorAdapter_Submit(t *testing.T) {
	// Stub validators
	oldValidate := validateJob
	oldSubmit := submitAndWaitTerminal
	validateJob = func(string) error { return nil }
	submitAndWaitTerminal = func(string, time.Duration) error { return nil }
	defer func() { validateJob = oldValidate; submitAndWaitTerminal = oldSubmit }()

	r := &TransflowRunner{workspaceDir: t.TempDir()}
	x := NewTransformationExecutorAdapter(r)
	diff := r.workspaceDir + "/out/diff.patch"
	params := ORWSubmitParams{SeaweedURL: "http://filer", ExecID: "e", BranchID: "b", StepID: "s", RunID: "job-1", SubmittedHCLPath: r.workspaceDir + "/hcl.hcl", DiffPath: diff, Timeout: time.Second}
	// touch hcl file
	if err := os.WriteFile(params.SubmittedHCLPath, []byte("job { }"), 0644); err != nil {
		t.Fatal(err)
	}
	// stub downloadToFileFn to create diff
	oldDL := downloadToFileFn
	downloadToFileFn = func(url, path string) error { return os.WriteFile(path, []byte("diff"), 0644) }
	defer func() { downloadToFileFn = oldDL }()

	out, err := x.SubmitORWAndFetchDiff(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != diff {
		t.Fatalf("unexpected diff path: %s", out)
	}
}

// HealerFanoutAdapter should delegate to HealingOrchestrator
type fakeHealer struct {
	called  bool
	winner  BranchResult
	results []BranchResult
	err     error
}

func (f *fakeHealer) RunFanout(ctx context.Context, runCtx any, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error) {
	f.called = true
	return f.winner, f.results, f.err
}

func TestHealerFanoutAdapter_Delegates(t *testing.T) {
	fh := &fakeHealer{winner: BranchResult{ID: "win", Status: "completed"}, results: []BranchResult{{ID: "win", Status: "completed"}}}
	fo := HealerFanoutAdapter{H: fh}
	w, all, err := fo.RunHealingFanout(context.Background(), nil, []BranchSpec{{ID: "a"}}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fh.called {
		t.Fatalf("expected healer RunFanout to be called")
	}
	if w.ID != "win" || w.Status != "completed" {
		t.Fatalf("unexpected winner: %+v", w)
	}
	if len(all) != 1 || all[0].ID != "win" {
		t.Fatalf("unexpected results: %+v", all)
	}
}
