package handlers

import (
	"slices"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestFindDuplicateDstsUsesStoredEntryContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		kind      contracts.HydraFileKind
		entries   []string
		wantDups  []string
		wantError string
	}{
		{
			name:    "colon in destination is preserved",
			kind:    contracts.HydraFileIn,
			entries: []string{"abcdef0:/in/some:path"},
		},
		{
			name:     "normalized destinations collide",
			kind:     contracts.HydraFileOut,
			entries:  []string{"abcdef0:/out/some//path", "bbbbbbb:/out/some/path"},
			wantDups: []string{"/out/some/path"},
		},
		{
			name:      "missing separator is rejected",
			kind:      contracts.HydraFileIn,
			entries:   []string{"abcdef0"},
			wantError: "expected format shortHash:dst",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := findDuplicateDsts(tc.kind, tc.entries)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("findDuplicateDsts() error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("findDuplicateDsts() error = %v", err)
			}
			if !slices.Equal(got, tc.wantDups) {
				t.Errorf("findDuplicateDsts() = %v, want %v", got, tc.wantDups)
			}
		})
	}
}

func TestMutateClaimSpecRejectsMalformedStoredHydraEntry(t *testing.T) {
	t.Parallel()

	_, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:    []byte(`{"steps":[{"image":"img:latest"}]}`),
		job:     store.Job{ID: domaintypes.NewJobID(), Meta: []byte(`{}`)},
		jobType: domaintypes.JobTypeMig,
		hydraOverlays: map[string]*HydraJobConfig{
			"mig": {In: []string{"abcdef0"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "merge hydra overlay into spec: spec.in: in entry \"abcdef0\": expected format shortHash:dst") {
		t.Fatalf("mutateClaimSpec() error = %v, want malformed stored-entry error", err)
	}
}

// ConfigHolder hydra overlay accessors.
func TestConfigHolder_HydraOverlays(t *testing.T) {
	t.Parallel()

	h := &ConfigHolder{}

	h.SetConfigIn("mig", []ConfigInEntry{{Entry: "abc1234567ab:/in/code.yaml", Dst: "/in/code.yaml", Section: "mig"}})

	overlays := h.GetHydraOverlays()
	if overlays == nil || overlays["mig"] == nil {
		t.Fatal("expected mig overlay")
	}
	if got := overlays["mig"].In; len(got) != 1 || got[0] != "abc1234567ab:/in/code.yaml" {
		t.Fatalf("mig In = %v, want [abc1234567ab:/in/code.yaml]", got)
	}

	// Verify returned overlays are defensive copies.
	overlays["mig"].In[0] = "mutated"
	overlaysAgain := h.GetHydraOverlays()
	if overlaysAgain["mig"].In[0] != "abc1234567ab:/in/code.yaml" {
		t.Fatal("expected In copy isolation")
	}
}

// Pipeline integration: full mutateClaimSpec with Hydra overlay.
func TestMutateClaimSpec_HydraOverlayInPipeline(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	out := mustMutateAndUnmarshal(t, claimSpecMutatorInput{
		spec:    []byte(`{"envs":{"EXISTING":"1"},"steps":[{"image":"img:latest"}]}`),
		job:     store.Job{ID: jobID, Meta: []byte(`{}`)},
		jobType: domaintypes.JobTypeMig,
		globalEnv: map[string][]GlobalEnvVar{
			"GLOBAL": {{Value: "g", Target: domaintypes.GlobalEnvTargetSteps}},
		},
		hydraOverlays: map[string]*HydraJobConfig{
			"mig": {
				In: []string{"abcdef0:/in/data.json"},
			},
		},
	})

	if got := out["job_id"]; got != jobID.String() {
		t.Errorf("job_id = %v, want %s", got, jobID.String())
	}
	assertEnvs(t, out, map[string]string{"EXISTING": "1", "GLOBAL": "g"}, nil, nil)
	assertSlices(t, firstStepMap(t, out), []sliceCheck{
		{"in", 1, "abcdef0:/in/data.json"},
	})
}
