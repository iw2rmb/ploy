package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestClaimJob_ServerAuthToken(t *testing.T) {
	for _, jobType := range []domaintypes.JobType{domaintypes.JobTypeMig, domaintypes.JobTypePreGate, domaintypes.JobTypePostGate} {
		for _, tc := range []struct {
			name       string
			server     string
			configured string
			run        string
			want       string
		}{
			{name: "server default", server: " server-token ", want: "server-token"},
			{name: "global override", server: "server-token", configured: "configured-token", want: "configured-token"},
			{name: "run override", server: "server-token", configured: "configured-token", run: "run-token", want: "run-token"},
			{name: "unset server", configured: "configured-token", want: "configured-token"},
			{name: "empty server", server: "  "},
		} {
			t.Run(string(jobType)+"/"+tc.name, func(t *testing.T) {
				t.Setenv("PLOY_AUTH_TOKEN", tc.server)
				spec := map[string]any{"steps": []any{map[string]any{"image": "test-image"}}}
				if tc.run != "" {
					spec["envs"] = map[string]string{"PLOY_AUTH_TOKEN": tc.run}
				}
				raw, err := json.Marshal(spec)
				if err != nil {
					t.Fatal(err)
				}
				f := newClaimJobFixture(t, claimJobFixtureOptions{jobType: jobType, specJSON: raw})
				if tc.configured != "" {
					f.config.SetGlobalEnvVar("PLOY_AUTH_TOKEN", GlobalEnvVar{
						Value: tc.configured, Target: domaintypes.GlobalEnvTargetNodes, Secret: true,
					})
				}
				configuredBefore := f.config.GetGlobalEnvAll()
				storedBefore := bytes.Clone(f.store.getSpec.val.Spec)
				rr := f.serve()
				assertStatus(t, rr, http.StatusOK)
				resp := decodeBody[map[string]any](t, rr)
				envs, _ := resp["spec"].(map[string]any)["envs"].(map[string]any)
				got, present := envs["PLOY_AUTH_TOKEN"]
				if tc.want == "" {
					if present {
						t.Fatal("claim unexpectedly contains PLOY_AUTH_TOKEN")
					}
				} else if got != tc.want {
					t.Fatal("claim PLOY_AUTH_TOKEN does not match the selected source")
				}
				if !bytes.Equal(f.store.getSpec.val.Spec, storedBefore) || !reflect.DeepEqual(f.config.GetGlobalEnvAll(), configuredBefore) {
					t.Fatal("claim changed stored specification or global configuration")
				}
			})
		}
	}
}

func TestClaimJob_MergesGlobalEnvIntoSpec(t *testing.T) {
	t.Parallel()

	f := newClaimJobFixture(t, claimJobFixtureOptions{
		specJSON: []byte(`{"envs":{"SECRET_BLOB":"per-run-secret","PER_RUN_ONLY":"value"}}`),
	})

	f.config.SetGlobalEnvVar("SECRET_BLOB", GlobalEnvVar{Value: "global-secret", Target: domaintypes.GlobalEnvTargetSteps, Secret: true})
	f.config.SetGlobalEnvVar("STEPS_SECRET", GlobalEnvVar{Value: "steps-secret-val", Target: domaintypes.GlobalEnvTargetSteps, Secret: true})
	f.config.SetGlobalEnvVar("CODEX_AUTH_JSON", GlobalEnvVar{Value: `{"token":"secret"}`, Target: domaintypes.GlobalEnvTargetSteps, Secret: true})
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

	// Per-run envs preserve precedence over global env values.
	if envs["SECRET_BLOB"] != "per-run-secret" {
		t.Fatalf("expected per-run SECRET_BLOB to win, got %v", envs["SECRET_BLOB"])
	}
	// Steps-target keys are injected for mig jobs.
	if envs["STEPS_SECRET"] != "steps-secret-val" {
		t.Fatalf("expected STEPS_SECRET to be injected, got %v", envs["STEPS_SECRET"])
	}
	if envs["CODEX_AUTH_JSON"] != `{"token":"secret"}` {
		t.Fatalf("expected CODEX_AUTH_JSON to be injected as an ordinary env var, got %v", envs["CODEX_AUTH_JSON"])
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

func TestClaimJob_ClaimKeepsBuildGateConfigUnchanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		jobType domaintypes.JobType
		spec    []byte
	}{
		{
			name:    "pre_gate preserves build_gate block",
			jobType: domaintypes.JobTypePreGate,
			spec: []byte(`{
				"steps":[{"image":"docker.io/acme/mig:latest"}],
				"build_gate":{
					"pre":{"stack":{"mode":"fallback","language":"java","tool":"maven","release":"17"}},
					"images":[{"stack":{"language":"java","tool":"maven","release":"17"},"image":"maven:jdk17"}]
				}
			}`),
		},
		{
			name:    "post_gate preserves build_gate block",
			jobType: domaintypes.JobTypePostGate,
			spec: []byte(`{
				"steps":[{"image":"docker.io/acme/mig:latest"}],
				"build_gate":{
					"post":{"stack":{"mode":"strict","language":"java","tool":"gradle","release":"21"}},
					"images":[{"stack":{"language":"java","tool":"gradle","release":"21"},"image":"gradle:jdk21"}]
				}
			}`),
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
			spec, ok := resp["spec"].(map[string]any)
			if !ok {
				t.Fatalf("expected spec object, got %T", resp["spec"])
			}

			var src map[string]any
			if err := json.Unmarshal(tc.spec, &src); err != nil {
				t.Fatalf("unmarshal source spec: %v", err)
			}
			wantBG, ok := src["build_gate"].(map[string]any)
			if !ok {
				t.Fatalf("source spec missing build_gate object")
			}
			gotBG, ok := spec["build_gate"].(map[string]any)
			if !ok {
				t.Fatalf("response spec missing build_gate object")
			}
			if !reflect.DeepEqual(gotBG, wantBG) {
				t.Fatalf("build_gate mutated:\n got=%v\nwant=%v", gotBG, wantBG)
			}
		})
	}
}
