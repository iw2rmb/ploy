package handlers

import (
	"net/http"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestClaimJob_MergesGlobalEnvIntoSpec(t *testing.T) {
	t.Parallel()

	f := newClaimJobFixture(t, claimJobFixtureOptions{
		specJSON: []byte(`{"env":{"CA_CERTS_PEM_BUNDLE":"per-run-cert","PER_RUN_ONLY":"value"}}`),
	})

	f.config.SetGlobalEnvVar("CA_CERTS_PEM_BUNDLE", GlobalEnvVar{Value: "global-cert", Target: domaintypes.GlobalEnvTargetSteps, Secret: true})
	f.config.SetGlobalEnvVar("CODEX_AUTH_JSON", GlobalEnvVar{Value: `{"token":"xxx"}`, Target: domaintypes.GlobalEnvTargetSteps, Secret: true})
	f.config.SetGlobalEnvVar("HEAL_ONLY", GlobalEnvVar{Value: "heal-env", Target: domaintypes.GlobalEnvTargetNodes, Secret: false})

	rr := f.serve()
	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[map[string]any](t, rr)
	spec, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec to be an object, got %T", resp["spec"])
	}
	env, ok := spec["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec.env to be an object, got %T", spec["env"])
	}

	if env["CA_CERTS_PEM_BUNDLE"] != "per-run-cert" {
		t.Fatalf("expected per-run CA_CERTS_PEM_BUNDLE to win, got %v", env["CA_CERTS_PEM_BUNDLE"])
	}
	if env["CODEX_AUTH_JSON"] != `{"token":"xxx"}` {
		t.Fatalf("expected CODEX_AUTH_JSON to be injected, got %v", env["CODEX_AUTH_JSON"])
	}
	if _, ok := env["HEAL_ONLY"]; ok {
		t.Fatalf("expected HEAL_ONLY not to be injected for mig job")
	}
	if env["PER_RUN_ONLY"] != "value" {
		t.Fatalf("expected PER_RUN_ONLY preserved, got %v", env["PER_RUN_ONLY"])
	}
}

func TestClaimJob_DoesNotMergeRepoGateProfileIntoGateSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		jobType   domaintypes.JobType
		spec      []byte
		wantPhase string
		wantCmd   string
		wantEnvK  string
		wantEnvV  string
	}{
		{
			name:      "pre_gate keeps spec unchanged without explicit gate_profile",
			jobType:   domaintypes.JobTypePreGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			wantPhase: "pre",
		},
		{
			name:      "post_gate keeps spec unchanged without explicit gate_profile",
			jobType:   domaintypes.JobTypePostGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			wantPhase: "post",
		},
		{
			name:      "re_gate keeps spec unchanged without explicit gate_profile",
			jobType:   domaintypes.JobTypeReGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			wantPhase: "post",
		},
		{
			name:    "explicit spec gate_profile is preserved",
			jobType: domaintypes.JobTypePreGate,
			spec: []byte(`{
				"steps":[{"image":"docker.io/acme/mod:latest"}],
				"build_gate":{"pre":{"gate_profile":{"command":"echo explicit","env":{"X":"1"}}}}
			}`),
			wantPhase: "pre",
			wantCmd:   "echo explicit",
			wantEnvK:  "X",
			wantEnvV:  "1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newClaimJobFixture(t, claimJobFixtureOptions{
				jobType:  tc.jobType,
				jobName:  "gate-0",
				specJSON: tc.spec,
			})
			rr := f.serve()
			assertStatus(t, rr, http.StatusOK)

			resp := decodeBody[map[string]any](t, rr)
			if got, ok := resp["repo_gate_profile_missing"].(bool); !ok || !got {
				t.Fatalf("expected repo_gate_profile_missing=true, got %v", resp["repo_gate_profile_missing"])
			}
			spec, ok := resp["spec"].(map[string]any)
			if !ok {
				t.Fatalf("expected spec object, got %T", resp["spec"])
			}
			bg, ok := spec["build_gate"].(map[string]any)
			if tc.wantCmd == "" {
				if ok {
					if phase, phaseOK := bg[tc.wantPhase].(map[string]any); phaseOK {
						if _, exists := phase["gate_profile"]; exists {
							t.Fatalf("did not expect build_gate.%s.gate_profile", tc.wantPhase)
						}
					}
				}
				return
			}
			if !ok {
				t.Fatalf("expected build_gate object, got %T", spec["build_gate"])
			}
			phase, ok := bg[tc.wantPhase].(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate.%s object, got %T", tc.wantPhase, bg[tc.wantPhase])
			}
			prepRaw, exists := phase["gate_profile"]
			if !exists {
				t.Fatalf("expected build_gate.%s.gate_profile", tc.wantPhase)
			}
			prep, ok := prepRaw.(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate.%s.gate_profile object, got %T", tc.wantPhase, prepRaw)
			}
			if got := prep["command"]; got != tc.wantCmd {
				t.Fatalf("build_gate.%s.gate_profile.command=%v, want %q", tc.wantPhase, got, tc.wantCmd)
			}
			env, ok := prep["env"].(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate.%s.gate_profile.env object, got %T", tc.wantPhase, prep["env"])
			}
			if got := env[tc.wantEnvK]; got != tc.wantEnvV {
				t.Fatalf("build_gate.%s.gate_profile.env[%s]=%v, want %q", tc.wantPhase, tc.wantEnvK, got, tc.wantEnvV)
			}
		})
	}
}
