package nodeagent

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestShouldCreateMR(t *testing.T) {
	tests := []struct {
		name           string
		terminalStatus string
		options        map[string]any
		want           bool
	}{
		{name: "success_flag_true", terminalStatus: "succeeded", options: map[string]any{"mr_on_success": true}, want: true},
		{name: "success_flag_false", terminalStatus: "succeeded", options: map[string]any{"mr_on_success": false}, want: false},
		{name: "success_flag_missing", terminalStatus: "succeeded", options: map[string]any{}, want: false},
		{name: "fail_flag_true", terminalStatus: "failed", options: map[string]any{"mr_on_fail": true}, want: true},
		{name: "fail_flag_false", terminalStatus: "failed", options: map[string]any{"mr_on_fail": false}, want: false},
		{name: "fail_flag_missing", terminalStatus: "failed", options: map[string]any{}, want: false},
		{name: "non_bool_values_ignored_success", terminalStatus: "succeeded", options: map[string]any{"mr_on_success": "true"}, want: false},
		{name: "non_bool_values_ignored_fail", terminalStatus: "failed", options: map[string]any{"mr_on_fail": "true"}, want: false},
		{name: "other_status_never_triggers", terminalStatus: "cancelled", options: map[string]any{"mr_on_success": true, "mr_on_fail": true}, want: false},
		{name: "gate_failure_with_mr_on_fail", terminalStatus: "failed", options: map[string]any{"mr_on_fail": true}, want: true},
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
	// Swap in fakes and restore on exit.
	origPusher := newPusher
	origMRClient := newMRClient
	defer func() {
		newPusher = origPusher
		newMRClient = origMRClient
	}()

	fakeP := &fakePusher{}
	fakeMR := &fakeMRClient{url: "http://example/mr/1"}
	newPusher = func() pusherIface { return fakeP }
	newMRClient = func() mrCreator { return fakeMR }

	// Minimal run controller.
	r := &runController{}
	req := StartRunRequest{
		RunID:     types.RunID("t-123"),
		RepoURL:   types.RepoURL("ssh://git@gitlab.example.com/acme/proj.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Options:   map[string]any{},
	}
	manifest := contracts.StepManifest{Options: map[string]any{
		"gitlab_pat":    "glpat-xyz",
		"gitlab_domain": "https://gitlab.example.com/",
	}}

	// Workspace dir is irrelevant for this orchestration test.
	ctx := context.Background()
	gotURL, err := r.createMR(ctx, req, manifest, ".")
	if err != nil {
		t.Fatalf("createMR error: %v", err)
	}
	if gotURL != "http://example/mr/1" {
		t.Fatalf("mr url = %q, want fixed fake", gotURL)
	}

	// Verify push options.
	if fakeP.opts.TargetRef != "ploy-t-123" {
		t.Fatalf("push target ref = %q, want ploy-t-123", fakeP.opts.TargetRef)
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
	if fakeMR.req.SourceBranch != "ploy-t-123" || fakeMR.req.TargetBranch != "main" {
		t.Fatalf("mr branches = %q -> %q, want ploy-t-123 -> main", fakeMR.req.SourceBranch, fakeMR.req.TargetBranch)
	}
}

// fakes
type fakePusher struct{ opts pushOptions }

func (f *fakePusher) Push(_ context.Context, opts pushOptions) error { f.opts = opts; return nil }

type fakeMRClient struct {
	url string
	req mrCreateReq
}

func (f *fakeMRClient) CreateMR(_ context.Context, req mrCreateReq) (string, error) {
	f.req = req
	return f.url, nil
}
