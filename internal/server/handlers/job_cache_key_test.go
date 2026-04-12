package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func sbomMetaForPhase(phase, cycle string) []byte {
	meta := contracts.NewMigJobMeta()
	meta.SBOM = &contracts.SBOMJobMetadata{
		Phase:     phase,
		CycleName: cycle,
		Role:      contracts.SBOMRoleInitial,
	}
	raw, _ := contracts.MarshalJobMeta(meta)
	return raw
}

func TestResolveHydraForCacheKey_HookUsesCyclePhaseCA(t *testing.T) {
	spec := &contracts.MigSpec{
		BuildGate: &contracts.BuildGateConfig{
			Pre:  &contracts.BuildGatePhaseConfig{CA: []string{"1111111"}},
			Post: &contracts.BuildGatePhaseConfig{CA: []string{"2222222"}},
		},
	}

	tests := []struct {
		name       string
		jobMetaRaw []byte
		wantCA     string
	}{
		{name: "pre_gate_hook", jobMetaRaw: sbomMetaForPhase(contracts.SBOMPhasePre, "pre-gate"), wantCA: "1111111"},
		{name: "post_gate_hook", jobMetaRaw: sbomMetaForPhase(contracts.SBOMPhasePost, "post-gate"), wantCA: "2222222"},
		{name: "re_gate_hook", jobMetaRaw: sbomMetaForPhase(contracts.SBOMPhasePost, "re-gate-1"), wantCA: "2222222"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inEntries, homeEntries, caEntries, err := resolveHydraForCacheKey(domaintypes.JobTypeHook, tt.jobMetaRaw, spec)
			if err != nil {
				t.Fatalf("resolveHydraForCacheKey(hook): %v", err)
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

func TestComputeJobCacheKey_UpstreamInputHashAffectsKey(t *testing.T) {
	t.Parallel()

	spec := []byte(`{"steps":[{"image":"ghcr.io/example/mig:1"}]}`)
	keyA, err := computeJobCacheKey(
		domaintypes.JobTypeMig,
		[]byte(`{"mig_step_index":0}`),
		"",
		"0123456789012345678901234567890123456789",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		spec,
	)
	if err != nil {
		t.Fatalf("computeJobCacheKey() with hash A error = %v", err)
	}
	keyB, err := computeJobCacheKey(
		domaintypes.JobTypeMig,
		[]byte(`{"mig_step_index":0}`),
		"",
		"0123456789012345678901234567890123456789",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		spec,
	)
	if err != nil {
		t.Fatalf("computeJobCacheKey() with hash B error = %v", err)
	}
	if keyA == keyB {
		t.Fatal("cache keys are equal for different upstream input hashes")
	}
}
