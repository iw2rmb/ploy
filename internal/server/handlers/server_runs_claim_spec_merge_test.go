package handlers

import (
	"net/http"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestClaimJob_MergesGlobalEnvIntoSpec(t *testing.T) {
	t.Parallel()

	f := newClaimJobFixture(t, claimJobFixtureOptions{
		specJSON: []byte(`{"envs":{"PLOY_CA_CERTS":"per-run-cert","PER_RUN_ONLY":"value"}}`),
	})

	f.config.SetGlobalEnvVar("PLOY_CA_CERTS", GlobalEnvVar{Value: "global-cert", Target: domaintypes.GlobalEnvTargetSteps, Secret: true})
	f.config.SetGlobalEnvVar("STEPS_SECRET", GlobalEnvVar{Value: "steps-secret-val", Target: domaintypes.GlobalEnvTargetSteps, Secret: true})
	f.config.SetGlobalEnvVar("NODES_FALLBACK", GlobalEnvVar{Value: "nodes-env", Target: domaintypes.GlobalEnvTargetNodes, Secret: false})
	f.config.SetGlobalEnvVar("SERVER_ONLY", GlobalEnvVar{Value: "server-env", Target: domaintypes.GlobalEnvTargetServer, Secret: false})

	rr := f.serve()
	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[map[string]any](t, rr)
	spec, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec to be an object, got %T", resp["spec"])
	}
	envs, ok := spec["envs"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec.envs to be an object, got %T", spec["envs"])
	}

	// Per-run envs for special key (PLOY_CA_CERTS) preserved in envs
	// (per-run values always win; the special-key filter only affects
	// global env overlay, not per-run spec values).
	if envs["PLOY_CA_CERTS"] != "per-run-cert" {
		t.Fatalf("expected per-run PLOY_CA_CERTS to win, got %v", envs["PLOY_CA_CERTS"])
	}
	// Steps-target non-special key injected for mig job.
	if envs["STEPS_SECRET"] != "steps-secret-val" {
		t.Fatalf("expected STEPS_SECRET to be injected, got %v", envs["STEPS_SECRET"])
	}
	// Special env keys with global env target must NOT be injected as raw envs.
	// They are file-backed and migrated to typed Hydra fields.
	if _, ok := envs["CODEX_AUTH_JSON"]; ok {
		t.Fatalf("expected special key CODEX_AUTH_JSON not to be injected as env var")
	}
	// Nodes-target provides fallback for all job types.
	if envs["NODES_FALLBACK"] != "nodes-env" {
		t.Fatalf("expected NODES_FALLBACK to be injected as fallback, got %v", envs["NODES_FALLBACK"])
	}
	// Server-target is not injected into job specs.
	if _, ok := envs["SERVER_ONLY"]; ok {
		t.Fatalf("expected SERVER_ONLY not to be injected for mig job")
	}
	if envs["PER_RUN_ONLY"] != "value" {
		t.Fatalf("expected PER_RUN_ONLY preserved, got %v", envs["PER_RUN_ONLY"])
	}
}

func TestClaimJob_JobTargetOverridesNodesTarget(t *testing.T) {
	t.Parallel()

	f := newClaimJobFixture(t, claimJobFixtureOptions{
		specJSON: []byte(`{}`),
	})

	// Same key with both nodes and steps targets — steps should win for mig job.
	f.config.SetGlobalEnvVar("SHARED_KEY", GlobalEnvVar{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes})
	f.config.SetGlobalEnvVar("SHARED_KEY", GlobalEnvVar{Value: "steps-val", Target: domaintypes.GlobalEnvTargetSteps})

	rr := f.serve()
	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[map[string]any](t, rr)
	spec := resp["spec"].(map[string]any)
	envs := spec["envs"].(map[string]any)

	if envs["SHARED_KEY"] != "steps-val" {
		t.Fatalf("expected steps-target to override nodes-target, got %v", envs["SHARED_KEY"])
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
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}]}`),
			wantPhase: "pre",
		},
		{
			name:      "post_gate keeps spec unchanged without explicit gate_profile",
			jobType:   domaintypes.JobTypePostGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}]}`),
			wantPhase: "post",
		},
		{
			name:      "re_gate keeps spec unchanged without explicit gate_profile",
			jobType:   domaintypes.JobTypeReGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}]}`),
			wantPhase: "post",
		},
		{
			name:    "explicit spec gate_profile is preserved",
			jobType: domaintypes.JobTypePreGate,
			spec: []byte(`{
				"steps":[{"image":"docker.io/acme/mig:latest"}],
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
