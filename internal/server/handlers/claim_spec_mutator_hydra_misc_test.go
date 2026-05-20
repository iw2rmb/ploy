package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// hydraExtractDst uses first-colon split aligned with Hydra parser semantics.
func TestHydraExtractDst_FirstColonSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		entry string
		want  string
	}{
		{
			name:  "in simple",
			field: "in",
			entry: "abcdef0:/in/data.json",
			want:  "/in/data.json",
		},
		{
			name:  "out simple",
			field: "out",
			entry: "abcdef0:/out/results",
			want:  "/out/results",
		},
		{
			name:  "in with colon in destination",
			field: "in",
			entry: "abcdef0:/in/some:path",
			want:  "/in/some:path",
		},
		{
			name:  "out with colon in destination",
			field: "out",
			entry: "abcdef0:/out/some:path",
			want:  "/out/some:path",
		},
		{
			name:  "home simple rw",
			field: "home",
			entry: "abcdef0:.config/app",
			want:  ".config/app",
		},
		{
			name:  "home simple ro",
			field: "home",
			entry: "abcdef0:.config/app:ro",
			want:  ".config/app",
		},
		{
			name:  "home with colon in destination",
			field: "home",
			entry: "abcdef0:.config/some:dir",
			want:  ".config/some:dir",
		},
		{
			name:  "home double slash cleaned",
			field: "home",
			entry: "abcdef0:.config//app",
			want:  ".config/app",
		},
		{
			name:  "no colon returns full entry for in",
			field: "in",
			entry: "nocolon",
			want:  "nocolon",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hydraExtractDst(tc.field, tc.entry)
			if got != tc.want {
				t.Errorf("hydraExtractDst(%q, %q) = %q, want %q", tc.field, tc.entry, got, tc.want)
			}
		})
	}
}

// ConfigHolder hydra overlay accessors.
func TestConfigHolder_HydraOverlays(t *testing.T) {
	t.Parallel()

	h := &ConfigHolder{}

	h.SetConfigHome("mig", []ConfigHomeEntry{{Entry: "abc1234567ab:.codex/auth.json:ro", Dst: ".codex/auth.json", Section: "mig"}})
	h.SetConfigIn("mig", []ConfigInEntry{{Entry: "abc1234567ab:/in/code.yaml", Dst: "/in/code.yaml", Section: "mig"}})

	overlays := h.GetHydraOverlays()
	if overlays == nil || overlays["mig"] == nil {
		t.Fatal("expected mig overlay")
	}
	if got := overlays["mig"].Home; len(got) != 1 || got[0] != "abc1234567ab:.codex/auth.json:ro" {
		t.Fatalf("mig Home = %v, want [abc1234567ab:.codex/auth.json:ro]", got)
	}
	if got := overlays["mig"].In; len(got) != 1 || got[0] != "abc1234567ab:/in/code.yaml" {
		t.Fatalf("mig In = %v, want [abc1234567ab:/in/code.yaml]", got)
	}

	// Verify returned overlays are defensive copies.
	overlays["mig"].Home[0] = "mutated"
	overlays["mig"].In[0] = "mutated"
	overlaysAgain := h.GetHydraOverlays()
	if overlaysAgain["mig"].Home[0] != "abc1234567ab:.codex/auth.json:ro" {
		t.Fatal("expected Home copy isolation")
	}
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
				In: []string{"/data:/in/data.json"},
			},
		},
	})

	if got := out["job_id"]; got != jobID.String() {
		t.Errorf("job_id = %v, want %s", got, jobID.String())
	}
	assertEnvs(t, out, map[string]string{"EXISTING": "1", "GLOBAL": "g"}, nil, nil)
	assertSlices(t, firstStepMap(t, out), []sliceCheck{
		{"in", 1, "/data:/in/data.json"},
	})
}
