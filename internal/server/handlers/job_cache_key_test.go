package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestResolveHydraForCacheKey_HookUsesCyclePhaseCA(t *testing.T) {
	spec := &contracts.MigSpec{
		BuildGate: &contracts.BuildGateConfig{
			Pre:  &contracts.BuildGatePhaseConfig{CA: []string{"1111111"}},
			Post: &contracts.BuildGatePhaseConfig{CA: []string{"2222222"}},
		},
	}

	tests := []struct {
		name    string
		jobName string
		wantCA  string
	}{
		{name: "pre_gate_hook", jobName: "pre-gate-hook-000", wantCA: "1111111"},
		{name: "post_gate_hook", jobName: "post-gate-hook-000", wantCA: "2222222"},
		{name: "re_gate_hook", jobName: "re-gate-1-hook-000", wantCA: "2222222"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inEntries, homeEntries, caEntries, err := resolveHydraForCacheKey(domaintypes.JobTypeHook, tt.jobName, spec)
			if err != nil {
				t.Fatalf("resolveHydraForCacheKey(hook, %q): %v", tt.jobName, err)
			}
			if len(inEntries) != 0 {
				t.Fatalf("in entries length = %d, want 0", len(inEntries))
			}
			if len(homeEntries) != 0 {
				t.Fatalf("home entries length = %d, want 0", len(homeEntries))
			}
			if len(caEntries) != 1 || caEntries[0] != tt.wantCA {
				t.Fatalf("ca entries = %v, want [%s]", caEntries, tt.wantCA)
			}
		})
	}
}
