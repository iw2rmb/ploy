package contracts

import (
	"encoding/json"
	"testing"
)

func TestMigSpec_RoundTrip(t *testing.T) {
	mrOnSuccess := true
	original := &MigSpec{
		Steps: []MigStep{{
			Image:   JobImage{Universal: "docker.io/user/mig:latest"},
			Command: CommandSpec{Shell: "echo hello"},
			Env:     map[string]string{"FOO": "bar"},
		}},
		BuildGate:   &BuildGateConfig{Enabled: true},
		GitLabPAT:   "secret",
		MROnSuccess: &mrOnSuccess,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	parsed, err := ParseMigSpecJSON(data)
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
	}

	if parsed.Steps[0].Image.Universal != original.Steps[0].Image.Universal {
		t.Errorf("image = %q, want %q", parsed.Steps[0].Image.Universal, original.Steps[0].Image.Universal)
	}
	if parsed.Steps[0].Command.Shell != original.Steps[0].Command.Shell {
		t.Errorf("command.Shell = %q, want %q", parsed.Steps[0].Command.Shell, original.Steps[0].Command.Shell)
	}
	if parsed.BuildGate == nil || !parsed.BuildGate.Enabled {
		t.Errorf("build_gate.enabled should be true")
	}
	if parsed.GitLabPAT != "secret" {
		t.Errorf("gitlab_pat = %q, want %q", parsed.GitLabPAT, "secret")
	}
}

// TestMigSpec_RoundTrip_MultiStep tests round-trip for multi-step specs.
func TestMigSpec_RoundTrip_MultiStep(t *testing.T) {
	original := &MigSpec{
		Steps: []MigStep{
			{Name: "step-1", Image: JobImage{Universal: "mod1:latest"}},
			{Name: "step-2", Image: JobImage{ByStack: map[MigStack]string{
				MigStackDefault:    "mod2:default",
				MigStackJavaMaven:  "mod2:maven",
				MigStackJavaGradle: "mod2:gradle",
			}}},
		},
		BuildGate: &BuildGateConfig{
			Healing: &HealingSpec{
				ByErrorKind: map[string]HealingActionSpec{
					"infra": {
						Retries: 2,
						Image:   JobImage{Universal: "codex:latest"},
					},
				},
			},
			Router: &RouterSpec{
				Image: JobImage{Universal: "router:latest"},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	parsed, err := ParseMigSpecJSON(data)
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
	}

	if len(parsed.Steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(parsed.Steps))
	}
	if parsed.Steps[0].Name != "step-1" {
		t.Errorf("steps[0].name = %q, want %q", parsed.Steps[0].Name, "step-1")
	}
	if !parsed.Steps[1].Image.IsStackSpecific() {
		t.Errorf("steps[1].image should be stack-specific")
	}
	if parsed.BuildGate == nil || parsed.BuildGate.Healing == nil || parsed.BuildGate.Healing.ByErrorKind["infra"].Retries != 2 {
		t.Errorf("build_gate.healing.retries should be 2")
	}
}

// TestCommandSpec_ToSlice tests command conversion to slice.
