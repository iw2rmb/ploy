package nodeagent

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/nodeagent/gitlab"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestShouldCreateMR verifies MR creation logic based on v1 job status values.
// v1 uses capitalized job status values: Success, Fail, Cancelled.
func TestShouldCreateMR(t *testing.T) {
	tests := []struct {
		name           string
		terminalStatus string
		options        map[string]any
		want           bool
	}{
		{name: "success_flag_true", terminalStatus: types.JobStatusSuccess.String(), options: map[string]any{"mr_on_success": true}, want: true},
		{name: "success_flag_false", terminalStatus: types.JobStatusSuccess.String(), options: map[string]any{"mr_on_success": false}, want: false},
		{name: "success_flag_missing", terminalStatus: types.JobStatusSuccess.String(), options: map[string]any{}, want: false},
		{name: "fail_flag_true", terminalStatus: types.JobStatusFail.String(), options: map[string]any{"mr_on_fail": true}, want: true},
		{name: "fail_flag_false", terminalStatus: types.JobStatusFail.String(), options: map[string]any{"mr_on_fail": false}, want: false},
		{name: "fail_flag_missing", terminalStatus: types.JobStatusFail.String(), options: map[string]any{}, want: false},
		{name: "non_bool_values_ignored_success", terminalStatus: types.JobStatusSuccess.String(), options: map[string]any{"mr_on_success": "true"}, want: false},
		{name: "non_bool_values_ignored_fail", terminalStatus: types.JobStatusFail.String(), options: map[string]any{"mr_on_fail": "true"}, want: false},
		{name: "other_status_never_triggers", terminalStatus: types.JobStatusCancelled.String(), options: map[string]any{"mr_on_success": true, "mr_on_fail": true}, want: false},
		{name: "gate_failure_with_mr_on_fail", terminalStatus: types.JobStatusFail.String(), options: map[string]any{"mr_on_fail": true}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := contracts.StepManifest{
				ID:      types.StepID("test-step"),
				Name:    "Test Step",
				Image:   "test:latest",
				Inputs:  []contracts.StepInput{{Name: "test", MountPath: "/test", Mode: contracts.StepInputModeReadOnly, SnapshotCID: "cid"}},
				Options: tt.options,
			}
			got := shouldCreateMR(tt.terminalStatus, manifest)
			if got != tt.want {
				t.Fatalf("shouldCreateMR(%q, %v) = %v, want %v", tt.terminalStatus, tt.options, got, tt.want)
			}
		})
	}
}

func TestBuildPushRemoteURL(t *testing.T) {
	t.Run("https_repo_passthrough", func(t *testing.T) {
		got, err := buildPushRemoteURL("https://gitlab.com/acme/proj.git", "gitlab.com", "acme%2Fproj")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "https://gitlab.com/acme/proj.git" {
			t.Fatalf("got %q, want https passthrough", got)
		}
	})

	t.Run("ssh_repo_builds_https_using_domain", func(t *testing.T) {
		got, err := buildPushRemoteURL("ssh://git@gitlab.example.com/group/sub/proj.git", "gitlab.example.com", "group%2Fsub%2Fproj")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "https://gitlab.example.com/group/sub/proj.git" {
			t.Fatalf("got %q, want https://gitlab.example.com/group/sub/proj.git", got)
		}
	})

	t.Run("file_repo_unsupported", func(t *testing.T) {
		if _, err := buildPushRemoteURL("file:///tmp/repo.git", "gitlab.com", "acme%2Fproj"); err == nil {
			t.Fatalf("expected error for file:// repo, got nil")
		}
	})
}

func TestCreateMR_OrchestratesPushAndMR(t *testing.T) {
	fakeP := &fakePusher{}
	fakeMR := &fakeMRClient{url: "http://example/mr/1"}

	r := &runController{
		newPusher:   func() git.Pusher { return fakeP },
		newMRClient: func() mrCreator { return fakeMR },
	}
	req := StartRunRequest{
		RunID:        types.RunID("t-123"),
		RepoURL:      types.RepoURL("ssh://git@gitlab.example.com/acme/proj.git"),
		BaseRef:      types.GitRef("main"),
		TargetRef:    types.GitRef("feature"),
		TypedOptions: RunOptions{},
	}
	manifest := contracts.StepManifest{Options: map[string]any{
		"gitlab_pat":    "glpat-xyz",
		"gitlab_domain": "https://gitlab.example.com/",
	}}

	// Workspace dir is irrelevant for this orchestration test.
	// Use a temp dir for workspace to avoid committing to the project repo.
	workspace := t.TempDir()
	initGitRepo(t, workspace)
	ctx := context.Background()
	gotURL, err := r.createMR(ctx, req, manifest, workspace)
	if err != nil {
		t.Fatalf("createMR error: %v", err)
	}
	if gotURL != "http://example/mr/1" {
		t.Fatalf("mr url = %q, want fixed fake", gotURL)
	}

	// Verify push options.
	wantBranch := req.TargetRef.String()
	if fakeP.opts.TargetRef != wantBranch {
		t.Fatalf("push target ref = %q, want %q", fakeP.opts.TargetRef, wantBranch)
	}
	if fakeP.opts.RemoteURL != "https://gitlab.example.com/acme/proj.git" {
		t.Fatalf("push remote = %q, want https://gitlab.example.com/acme/proj.git", fakeP.opts.RemoteURL)
	}

	// Verify MR request parameters captured by fake client.
	if fakeMR.req.Domain != "gitlab.example.com" {
		t.Fatalf("mr domain = %q, want gitlab.example.com", fakeMR.req.Domain)
	}
	if fakeMR.req.ProjectID != "acme%2Fproj" {
		t.Fatalf("mr project_id = %q, want acme%%2Fproj", fakeMR.req.ProjectID)
	}
	if fakeMR.req.SourceBranch != wantBranch || fakeMR.req.TargetBranch != req.BaseRef.String() {
		t.Fatalf("mr branches = %q -> %q, want %q -> %q", fakeMR.req.SourceBranch, fakeMR.req.TargetBranch, wantBranch, req.BaseRef.String())
	}
}

func TestCreateMR_DefaultsSourceBranchWhenTargetRefEmpty(t *testing.T) {
	fakeP := &fakePusher{}
	fakeMR := &fakeMRClient{url: "http://example/mr/2"}

	runID := types.RunID("t-empty-target")
	r := &runController{
		newPusher:   func() git.Pusher { return fakeP },
		newMRClient: func() mrCreator { return fakeMR },
	}
	req := StartRunRequest{
		RunID:        runID,
		RepoURL:      types.RepoURL("ssh://git@gitlab.example.com/acme/proj.git"),
		BaseRef:      types.GitRef("main"),
		TargetRef:    "", // Unspecified target ref.
		TypedOptions: RunOptions{},
	}
	manifest := contracts.StepManifest{Options: map[string]any{
		"gitlab_pat":    "glpat-xyz",
		"gitlab_domain": "gitlab.example.com",
	}}

	// Use a temp dir for workspace to avoid committing to the project repo.
	workspace := t.TempDir()
	initGitRepo(t, workspace)
	ctx := context.Background()
	gotURL, err := r.createMR(ctx, req, manifest, workspace)
	if err != nil {
		t.Fatalf("createMR error: %v", err)
	}
	if gotURL != "http://example/mr/2" {
		t.Fatalf("mr url = %q, want fixed fake", gotURL)
	}

	// Expect default branch ploy/<run-id> when no run name is provided.
	wantBranch := "ploy/" + runID.String()
	if fakeP.opts.TargetRef != wantBranch {
		t.Fatalf("push target ref = %q, want %q", fakeP.opts.TargetRef, wantBranch)
	}
	if fakeMR.req.SourceBranch != wantBranch {
		t.Fatalf("mr source branch = %q, want %q", fakeMR.req.SourceBranch, wantBranch)
	}
	if fakeMR.req.TargetBranch != req.BaseRef.String() {
		t.Fatalf("mr target branch = %q, want %q", fakeMR.req.TargetBranch, req.BaseRef.String())
	}
}

func TestCreateMR_DefaultsSourceBranchFromRunName(t *testing.T) {
	fakeP := &fakePusher{}
	fakeMR := &fakeMRClient{url: "http://example/mr/3"}

	runID := types.RunID("t-named-run")
	runName := "batch-foo"

	r := &runController{
		newPusher:   func() git.Pusher { return fakeP },
		newMRClient: func() mrCreator { return fakeMR },
	}
	req := StartRunRequest{
		RunID:        runID,
		Name:         runName,
		RepoURL:      types.RepoURL("ssh://git@gitlab.example.com/acme/proj.git"),
		BaseRef:      types.GitRef("main"),
		TargetRef:    "", // Unspecified target ref.
		TypedOptions: RunOptions{},
	}
	manifest := contracts.StepManifest{Options: map[string]any{
		"gitlab_pat":    "glpat-xyz",
		"gitlab_domain": "gitlab.example.com",
	}}

	// Use a temp dir for workspace to avoid committing to the project repo.
	workspace := t.TempDir()
	initGitRepo(t, workspace)
	ctx := context.Background()
	gotURL, err := r.createMR(ctx, req, manifest, workspace)
	if err != nil {
		t.Fatalf("createMR error: %v", err)
	}
	if gotURL != "http://example/mr/3" {
		t.Fatalf("mr url = %q, want fixed fake", gotURL)
	}

	// Expect default branch ploy/<run_name> when run name is provided.
	wantBranch := "ploy/" + runName
	if fakeP.opts.TargetRef != wantBranch {
		t.Fatalf("push target ref = %q, want %q", fakeP.opts.TargetRef, wantBranch)
	}
	if fakeMR.req.SourceBranch != wantBranch {
		t.Fatalf("mr source branch = %q, want %q", fakeMR.req.SourceBranch, wantBranch)
	}
	if fakeMR.req.TargetBranch != req.BaseRef.String() {
		t.Fatalf("mr target branch = %q, want %q", fakeMR.req.TargetBranch, req.BaseRef.String())
	}
}

// fakes
type fakePusher struct{ opts git.PushOptions }

func (f *fakePusher) Push(_ context.Context, opts git.PushOptions) error { f.opts = opts; return nil }

type fakeMRClient struct {
	url string
	req gitlab.MRCreateRequest
}

func (f *fakeMRClient) CreateMR(_ context.Context, req gitlab.MRCreateRequest) (string, error) {
	f.req = req
	return f.url, nil
}
