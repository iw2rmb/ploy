package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestClaimJob_HealMergesSelectedErrorKindAndExpectedArtifacts(t *testing.T) {
	t.Parallel()

	f := newClaimJobFixture(t, claimJobFixtureOptions{
		jobType: domaintypes.JobTypeHeal,
		jobName: "heal-1-0",
		specJSON: []byte(`{
			"steps":[{"image":"docker.io/acme/mod:latest"}],
			"build_gate":{
				"healing":{
					"by_error_kind":{
						"infra":{"retries":2,"image":"docker.io/acme/heal:latest"}
					}
				},
				"router":{"image":"docker.io/acme/router:latest"}
			}
		}`),
		jobImage: "docker.io/acme/heal:latest",
		jobMeta:  []byte(`{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}}`),
	})
	rr := f.serve()
	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[map[string]any](t, rr)
	specObj, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec object, got %T", resp["spec"])
	}
	bg, ok := specObj["build_gate"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate object, got %T", specObj["build_gate"])
	}
	healing, ok := bg["healing"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate.healing object, got %T", bg["healing"])
	}
	if got := healing["selected_error_kind"]; got != "infra" {
		t.Fatalf("build_gate.healing.selected_error_kind=%v, want infra", got)
	}
	paths, ok := specObj["artifact_paths"].([]any)
	if !ok {
		t.Fatalf("expected artifact_paths array, got %T", specObj["artifact_paths"])
	}
	found := false
	for _, p := range paths {
		if s, ok := p.(string); ok && s == "/out/gate-profile-candidate.json" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected artifact_paths to include /out/gate-profile-candidate.json, got %#v", paths)
	}
	envObj, ok := specObj["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec.env object, got %T", specObj["env"])
	}
	schemaRaw, ok := envObj[contracts.GateProfileSchemaJSONEnv].(string)
	if !ok || strings.TrimSpace(schemaRaw) == "" {
		t.Fatalf("expected %s in spec.env, got %v", contracts.GateProfileSchemaJSONEnv, envObj[contracts.GateProfileSchemaJSONEnv])
	}
	if !json.Valid([]byte(schemaRaw)) {
		t.Fatalf("expected %s to be valid JSON", contracts.GateProfileSchemaJSONEnv)
	}
	rc, ok := resp["recovery_context"].(map[string]any)
	if !ok {
		t.Fatalf("expected recovery_context object, got %T", resp["recovery_context"])
	}
	if got := rc["selected_error_kind"]; got != "infra" {
		t.Fatalf("recovery_context.selected_error_kind=%v, want infra", got)
	}
	if got := rc["resolved_healing_image"]; got != "docker.io/acme/heal:latest" {
		t.Fatalf("recovery_context.resolved_healing_image=%v, want docker.io/acme/heal:latest", got)
	}
	if _, ok := rc["gate_profile_schema_json"].(string); !ok {
		t.Fatalf("expected recovery_context.gate_profile_schema_json string, got %T", rc["gate_profile_schema_json"])
	}
}

func TestClaimJob_HealNonInfraDoesNotInjectSchemaEnv(t *testing.T) {
	t.Parallel()

	f := newClaimJobFixture(t, claimJobFixtureOptions{
		jobType: domaintypes.JobTypeHeal,
		jobName: "heal-1-0",
		specJSON: []byte(`{
			"steps":[{"image":"docker.io/acme/mod:latest"}],
			"build_gate":{
				"healing":{
					"by_error_kind":{
						"infra":{"retries":2,"image":"docker.io/acme/heal:latest"},
						"code":{"retries":1,"image":"docker.io/acme/heal:latest"}
					}
				}
			}
		}`),
		jobImage: "docker.io/acme/heal:latest",
		jobMeta:  []byte(`{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"code","strategy_id":"code-default"}}`),
	})
	rr := f.serve()
	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[map[string]any](t, rr)
	specObj, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec object, got %T", resp["spec"])
	}
	envObj, _ := specObj["env"].(map[string]any)
	if envObj != nil {
		if _, ok := envObj[contracts.GateProfileSchemaJSONEnv]; ok {
			t.Fatalf("did not expect %s for non-infra heal", contracts.GateProfileSchemaJSONEnv)
		}
	}
	rc, ok := resp["recovery_context"].(map[string]any)
	if !ok {
		t.Fatalf("expected recovery_context object, got %T", resp["recovery_context"])
	}
	if got := rc["selected_error_kind"]; got != "code" {
		t.Fatalf("recovery_context.selected_error_kind=%v, want code", got)
	}
	if _, ok := rc["gate_profile_schema_json"]; ok {
		t.Fatalf("did not expect recovery_context.gate_profile_schema_json for non-infra heal")
	}
}

func TestClaimJob_DepsCompatRecoveryContextIncludesEndpointAndBumps(t *testing.T) {
	t.Parallel()

	gateJobID := domaintypes.NewJobID()

	f := newClaimJobFixture(t, claimJobFixtureOptions{
		jobType:  domaintypes.JobTypeHeal,
		jobName:  "heal-1-0",
		jobImage: "docker.io/acme/heal:latest",
		specJSON: []byte(`{
			"steps":[{"image":"docker.io/acme/mod:latest"}],
			"build_gate":{
				"healing":{
					"by_error_kind":{
						"deps":{"retries":2,"image":"docker.io/acme/heal:latest"}
					}
				}
			}
		}`),
		jobMeta: []byte(`{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"deps","strategy_id":"deps-default","deps_bumps":{"org.slf4j:slf4j-api":"2.0.13","legacy:shim":null}}}`),
	})

	f.store.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:      gateJobID,
			RunID:   f.runID,
			RepoID:  f.repoID,
			Attempt: 1,
			JobType: domaintypes.JobTypePreGate,
			NextID:  &f.jobID,
			Meta:    []byte(`{"kind":"gate","gate":{"detected_stack":{"language":"java","release":"17","tool":"maven"}}}`),
		},
		{
			ID:      f.jobID,
			RunID:   f.runID,
			RepoID:  f.repoID,
			Attempt: 1,
			JobType: domaintypes.JobTypeHeal,
		},
	}

	rr := f.serve()
	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[map[string]any](t, rr)
	rc, ok := resp["recovery_context"].(map[string]any)
	if !ok {
		t.Fatalf("expected recovery_context object, got %T", resp["recovery_context"])
	}
	if got := rc["selected_error_kind"]; got != "deps" {
		t.Fatalf("recovery_context.selected_error_kind=%v, want deps", got)
	}
	if got := rc["deps_compat_endpoint"]; got != "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=" {
		t.Fatalf("recovery_context.deps_compat_endpoint=%v, want stack-prefilled endpoint", got)
	}
	depsBumps, ok := rc["deps_bumps"].(map[string]any)
	if !ok {
		t.Fatalf("expected recovery_context.deps_bumps object, got %T", rc["deps_bumps"])
	}
	if got := depsBumps["org.slf4j:slf4j-api"]; got != "2.0.13" {
		t.Fatalf("deps_bumps[org.slf4j:slf4j-api]=%v, want 2.0.13", got)
	}
	if got, ok := depsBumps["legacy:shim"]; !ok || got != nil {
		t.Fatalf("deps_bumps[legacy:shim]=%v (present=%v), want null", got, ok)
	}
}
