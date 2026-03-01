package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestBuildRouterManifest_InjectsPhaseAndLoopEnv(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID: types.RunID("run-router-env"),
		JobID: types.JobID("job-router-env"),
	}
	router := ModContainerSpec{
		Image: contracts.JobImage{Universal: "test/router:latest"},
		Env: map[string]string{
			"ROUTER_MODE": "classify",
		},
	}
	manifest, err := buildRouterManifest(req, router, contracts.ModStackUnknown, types.JobTypePreGate, "healing")
	if err != nil {
		t.Fatalf("buildRouterManifest() error = %v", err)
	}

	if got, want := manifest.Env["PLOY_GATE_PHASE"], "pre_gate"; got != want {
		t.Fatalf("PLOY_GATE_PHASE = %q, want %q", got, want)
	}
	if got, want := manifest.Env["PLOY_LOOP_KIND"], "healing"; got != want {
		t.Fatalf("PLOY_LOOP_KIND = %q, want %q", got, want)
	}
	if got, want := manifest.Env["ROUTER_MODE"], "classify"; got != want {
		t.Fatalf("ROUTER_MODE = %q, want %q", got, want)
	}
}

func TestBuildRouterManifest_ContextEnvOverridesUserValues(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID: types.RunID("run-router-env-override"),
		JobID: types.JobID("job-router-env-override"),
	}
	router := ModContainerSpec{
		Image: contracts.JobImage{Universal: "test/router:latest"},
		Env: map[string]string{
			"PLOY_GATE_PHASE": "post_gate",
			"PLOY_LOOP_KIND":  "custom",
		},
	}
	manifest, err := buildRouterManifest(req, router, contracts.ModStackUnknown, types.JobTypeReGate, "healing")
	if err != nil {
		t.Fatalf("buildRouterManifest() error = %v", err)
	}

	if got, want := manifest.Env["PLOY_GATE_PHASE"], "re_gate"; got != want {
		t.Fatalf("PLOY_GATE_PHASE = %q, want %q", got, want)
	}
	if got, want := manifest.Env["PLOY_LOOP_KIND"], "healing"; got != want {
		t.Fatalf("PLOY_LOOP_KIND = %q, want %q", got, want)
	}
}
