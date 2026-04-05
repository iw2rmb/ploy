package nodeagent

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestResolveHealingWorkspacePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     *contracts.RecoveryClaimContext
		wantPol workspaceChangePolicy
	}{
		{
			name:    "nil context defaults to require changes",
			ctx:     nil,
			wantPol: workspaceChangePolicyRequire,
		},
		{
			name: "infra (schema present) forbids changes",
			ctx: &contracts.RecoveryClaimContext{
				GateProfileSchemaJSON: `{"type":"object"}`,
			},
			wantPol: workspaceChangePolicyForbid,
		},
		{
			name: "code (no schema) requires changes",
			ctx: &contracts.RecoveryClaimContext{
				LoopKind: "healing",
			},
			wantPol: workspaceChangePolicyRequire,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveHealingWorkspacePolicy(tc.ctx)
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
