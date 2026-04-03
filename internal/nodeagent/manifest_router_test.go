package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestBuildRouterManifest_PhaseAndLoopEnv verifies that PLOY_GATE_PHASE,
// PLOY_LOOP_KIND, and user env vars are correctly injected or overridden.
func TestBuildRouterManifest_PhaseAndLoopEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		router    MigContainerSpec
		gatePhase types.JobType
		loopKind  string
		wantEnv   map[string]string
	}{
		{
			name: "injects phase and loop env alongside user env",
			router: MigContainerSpec{
				Image: contracts.JobImage{Universal: "test/router:latest"},
				Env:   map[string]string{"ROUTER_MODE": "classify"},
			},
			gatePhase: types.JobTypePreGate,
			loopKind:  "healing",
			wantEnv: map[string]string{
				"PLOY_GATE_PHASE": "pre_gate",
				"PLOY_LOOP_KIND":  "healing",
				"ROUTER_MODE":     "classify",
			},
		},
		{
			name: "context env overrides user values",
			router: MigContainerSpec{
				Image: contracts.JobImage{Universal: "test/router:latest"},
				Env: map[string]string{
					"PLOY_GATE_PHASE": "post_gate",
					"PLOY_LOOP_KIND":  "custom",
				},
			},
			gatePhase: types.JobTypeReGate,
			loopKind:  "healing",
			wantEnv: map[string]string{
				"PLOY_GATE_PHASE": "re_gate",
				"PLOY_LOOP_KIND":  "healing",
			},
		},
	}

	req := newStartRunRequest(
		withRunID("run-router-env"), withJobID("job-router-env"),
	)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manifest, err := buildRouterManifest(req, tc.router, contracts.MigStackUnknown, tc.gatePhase, tc.loopKind)
			if err != nil {
				t.Fatalf("buildRouterManifest() error = %v", err)
			}
			assertEnvContains(t, manifest.Envs, tc.wantEnv)
		})
	}
}

// TestBuildRouterManifest_AmataVsShellCommand verifies the router builder
// picks amata command when spec is present and falls through to shell otherwise.
func TestBuildRouterManifest_AmataVsShellCommand(t *testing.T) {
	t.Parallel()

	req := newStartRunRequest(
		withRunID("run-router-amata"), withJobID("job-router-amata"),
	)

	tests := []struct {
		name    string
		router  MigContainerSpec
		wantCmd []string
	}{
		{
			name: "amata spec selects amata command",
			router: MigContainerSpec{
				Image: contracts.JobImage{Universal: "test/router:latest"},
				Amata: &contracts.AmataRunSpec{Spec: "task: route"},
			},
			wantCmd: []string{"amata", "run", "/in/amata.yaml"},
		},
		{
			name: "nil amata uses shell command",
			router: MigContainerSpec{
				Image:   contracts.JobImage{Universal: "test/router:latest"},
				Command: contracts.CommandSpec{Shell: "codex exec"},
			},
			wantCmd: []string{"/bin/sh", "-c", "codex exec"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			manifest, err := buildRouterManifest(req, tc.router, contracts.MigStackUnknown, types.JobTypePreGate, "healing")
			if err != nil {
				t.Fatalf("buildRouterManifest() error = %v", err)
			}
			assertCommandEqual(t, manifest.Command, tc.wantCmd)
		})
	}
}
