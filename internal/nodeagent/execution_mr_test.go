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

func TestCreateMR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		targetRef string
		runName   string
		wantBranch string
		wantMRURL  string
	}{
		{
			name:       "orchestrates_push_and_mr",
			targetRef:  "feature",
			runName:    "",
			wantBranch: "feature",
			wantMRURL:  "http://example/mr/1",
		},
		{
			name:       "defaults_source_branch_when_target_ref_empty",
			targetRef:  "",
			runName:    "",
			wantBranch: "ploy/t-empty-target",
			wantMRURL:  "http://example/mr/2",
		},
		{
			name:       "defaults_source_branch_from_run_name",
			targetRef:  "",
			runName:    "batch-foo",
			wantBranch: "ploy/batch-foo",
			wantMRURL:  "http://example/mr/3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeP := &fakePusher{}
			fakeMR := &fakeMRClient{url: tt.wantMRURL}

			r := &runController{
				newPusher:   func() git.Pusher { return fakeP },
				newMRClient: func() mrCreator { return fakeMR },
			}
			req := StartRunRequest{
				RunID:        types.RunID("t-empty-target"),
				Name:         tt.runName,
				RepoURL:      types.RepoURL("ssh://git@gitlab.example.com/acme/proj.git"),
				BaseRef:      types.GitRef("main"),
				TargetRef:    types.GitRef(tt.targetRef),
				TypedOptions: RunOptions{},
			}
			manifest := contracts.StepManifest{Options: map[string]any{
				"gitlab_pat":    "glpat-xyz",
				"gitlab_domain": "gitlab.example.com",
			}}

			workspace := t.TempDir()
			initGitRepo(t, workspace)

			gotURL, err := r.createMR(context.Background(), req, manifest, workspace)
			if err != nil {
				t.Fatalf("createMR error: %v", err)
			}
			if gotURL != tt.wantMRURL {
				t.Fatalf("mr url = %q, want %q", gotURL, tt.wantMRURL)
			}

			// Verify push branch.
			if fakeP.opts.TargetRef != tt.wantBranch {
				t.Fatalf("push target ref = %q, want %q", fakeP.opts.TargetRef, tt.wantBranch)
			}
			if fakeP.opts.RemoteURL != "https://gitlab.example.com/acme/proj.git" {
				t.Fatalf("push remote = %q, want https://gitlab.example.com/acme/proj.git", fakeP.opts.RemoteURL)
			}

			// Verify MR request parameters.
			if fakeMR.req.Domain != "gitlab.example.com" {
				t.Fatalf("mr domain = %q, want gitlab.example.com", fakeMR.req.Domain)
			}
			if fakeMR.req.ProjectID != "acme%2Fproj" {
				t.Fatalf("mr project_id = %q, want acme%%2Fproj", fakeMR.req.ProjectID)
			}
			if fakeMR.req.SourceBranch != tt.wantBranch {
				t.Fatalf("mr source branch = %q, want %q", fakeMR.req.SourceBranch, tt.wantBranch)
			}
			if fakeMR.req.TargetBranch != "main" {
				t.Fatalf("mr target branch = %q, want main", fakeMR.req.TargetBranch)
			}
		})
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
