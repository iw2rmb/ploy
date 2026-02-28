package nodeagent

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestResolveHealingWorkspacePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    *contracts.HealingSpec
		wantPol workspaceChangePolicy
	}{
		{
			name:    "nil spec defaults to require changes",
			spec:    nil,
			wantPol: workspaceChangePolicyRequire,
		},
		{
			name: "infra forbids changes",
			spec: &contracts.HealingSpec{
				SelectedErrorKind: "infra",
			},
			wantPol: workspaceChangePolicyForbid,
		},
		{
			name: "code requires changes",
			spec: &contracts.HealingSpec{
				SelectedErrorKind: "code",
			},
			wantPol: workspaceChangePolicyRequire,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveHealingWorkspacePolicy(tc.spec)
			if got != tc.wantPol {
				t.Fatalf("resolveHealingWorkspacePolicy() = %q, want %q", got, tc.wantPol)
			}
		})
	}
}

func TestValidateWorkspacePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		policy      workspaceChangePolicy
		preStatus   string
		postStatus  string
		wantWarning string
		wantFail    bool
	}{
		{
			name:       "require changes passes when changed",
			policy:     workspaceChangePolicyRequire,
			preStatus:  "",
			postStatus: " M file.go",
		},
		{
			name:        "require changes fails when unchanged",
			policy:      workspaceChangePolicyRequire,
			preStatus:   "",
			postStatus:  "",
			wantWarning: "no_workspace_changes",
			wantFail:    true,
		},
		{
			name:       "forbid changes passes when unchanged",
			policy:     workspaceChangePolicyForbid,
			preStatus:  "",
			postStatus: "",
		},
		{
			name:        "forbid changes fails when changed",
			policy:      workspaceChangePolicyForbid,
			preStatus:   "",
			postStatus:  " M file.go",
			wantWarning: "unexpected_workspace_changes",
			wantFail:    true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotWarning, gotFail := validateWorkspacePolicy(tc.policy, tc.preStatus, tc.postStatus)
			if gotWarning != tc.wantWarning || gotFail != tc.wantFail {
				t.Fatalf("validateWorkspacePolicy() = (%q, %v), want (%q, %v)", gotWarning, gotFail, tc.wantWarning, tc.wantFail)
			}
		})
	}
}
