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

// TestBuildRouterManifest_AmataCommand verifies that when Router.Amata.Spec is set
// the manifest command resolves to amata run with ordered --set flags, and when
// Amata is nil the direct router command is used.
func TestBuildRouterManifest_AmataCommand(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID: types.RunID("run-router-amata"),
		JobID: types.JobID("job-router-amata"),
	}

	t.Run("amata_spec_selects_amata_command", func(t *testing.T) {
		t.Parallel()

		router := ModContainerSpec{
			Image:   contracts.JobImage{Universal: "test/router:latest"},
			Command: contracts.CommandSpec{Shell: "codex exec"},
			Amata: &contracts.AmataRunSpec{
				Spec: "task: route",
				Set: []contracts.AmataSetParam{
					{Param: "mode", Value: "classify"},
					{Param: "strict", Value: "true"},
				},
			},
		}
		manifest, err := buildRouterManifest(req, router, contracts.ModStackUnknown, types.JobTypePreGate, "healing")
		if err != nil {
			t.Fatalf("buildRouterManifest() error = %v", err)
		}
		want := []string{"amata", "run", "/in/amata.yaml", "--set", "mode=classify", "--set", "strict=true"}
		if len(manifest.Command) != len(want) {
			t.Fatalf("Command len: got %d, want %d: %v", len(manifest.Command), len(want), manifest.Command)
		}
		for i, v := range want {
			if manifest.Command[i] != v {
				t.Errorf("Command[%d]: got %q, want %q", i, manifest.Command[i], v)
			}
		}
	})

	t.Run("nil_amata_uses_direct_command", func(t *testing.T) {
		t.Parallel()

		router := ModContainerSpec{
			Image:   contracts.JobImage{Universal: "test/router:latest"},
			Command: contracts.CommandSpec{Shell: "codex exec"},
			Amata:   nil,
		}
		manifest, err := buildRouterManifest(req, router, contracts.ModStackUnknown, types.JobTypePreGate, "healing")
		if err != nil {
			t.Fatalf("buildRouterManifest() error = %v", err)
		}
		want := []string{"/bin/sh", "-c", "codex exec"}
		if len(manifest.Command) != len(want) {
			t.Fatalf("Command len: got %d, want %d: %v", len(manifest.Command), len(want), manifest.Command)
		}
		for i, v := range want {
			if manifest.Command[i] != v {
				t.Errorf("Command[%d]: got %q, want %q", i, manifest.Command[i], v)
			}
		}
	})

	t.Run("amata_empty_spec_falls_through_to_direct_command", func(t *testing.T) {
		t.Parallel()

		router := ModContainerSpec{
			Image:   contracts.JobImage{Universal: "test/router:latest"},
			Command: contracts.CommandSpec{Shell: "codex exec"},
			Amata:   &contracts.AmataRunSpec{Spec: ""},
		}
		manifest, err := buildRouterManifest(req, router, contracts.ModStackUnknown, types.JobTypePreGate, "healing")
		if err != nil {
			t.Fatalf("buildRouterManifest() error = %v", err)
		}
		want := []string{"/bin/sh", "-c", "codex exec"}
		if len(manifest.Command) != len(want) {
			t.Fatalf("Command len: got %d, want %d: %v", len(manifest.Command), len(want), manifest.Command)
		}
		for i, v := range want {
			if manifest.Command[i] != v {
				t.Errorf("Command[%d]: got %q, want %q", i, manifest.Command[i], v)
			}
		}
	})
}
