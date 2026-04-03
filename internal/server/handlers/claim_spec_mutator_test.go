package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestMutateClaimSpec_ReGateCandidateWinsOverRepoProfile(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	job := testMutatorJobWithRecoveryMeta(jobID, fmt.Sprintf(
		`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_validation_status":"%s","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"unit","unit":{"status":"passed","command":"echo candidate","env":{"SRC":"candidate"}},"build":{"status":"passed","command":"echo candidate build","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}}}`,
		contracts.RecoveryCandidateStatusValid,
	))

	repoProfile := []byte(`{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"unit","unit":{"status":"passed","command":"echo repo","env":{"SRC":"repo"}},"build":{"status":"passed","command":"echo repo build","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`)

	merged, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:            []byte(`{"envs":{"EXISTING":"1"}}`),
		job:             job,
		jobType:         domaintypes.JobTypeReGate,
		gitLab:          config.GitLabConfig{Token: "server-token", Domain: "https://gitlab.example.com"},
		globalEnv:       map[string][]GlobalEnvVar{"GLOBAL": {{Value: "g", Target: domaintypes.GlobalEnvTargetGates}}},
		repoGateProfile: repoProfile,
	})
	if err != nil {
		t.Fatalf("mutateClaimSpec: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}

	if got := out["job_id"]; got != jobID.String() {
		t.Fatalf("job_id=%v, want %s", got, jobID.String())
	}
	if got := out["gitlab_pat"]; got != "server-token" {
		t.Fatalf("gitlab_pat=%v, want server-token", got)
	}
	if got := out["gitlab_domain"]; got != "https://gitlab.example.com" {
		t.Fatalf("gitlab_domain=%v, want https://gitlab.example.com", got)
	}
	envs, ok := out["envs"].(map[string]any)
	if !ok {
		t.Fatalf("expected envs object, got %T", out["envs"])
	}
	if got := envs["EXISTING"]; got != "1" {
		t.Fatalf("envs.EXISTING=%v, want 1", got)
	}
	if got := envs["GLOBAL"]; got != "g" {
		t.Fatalf("envs.GLOBAL=%v, want g", got)
	}
	bg := out["build_gate"].(map[string]any)
	post := bg["post"].(map[string]any)
	gp := post["gate_profile"].(map[string]any)
	if got := gp["command"]; got != "echo candidate" {
		t.Fatalf("build_gate.post.gate_profile.command=%v, want echo candidate", got)
	}
	gpEnv := gp["env"].(map[string]any)
	if got := gpEnv["SRC"]; got != "candidate" {
		t.Fatalf("build_gate.post.gate_profile.env[SRC]=%v, want candidate", got)
	}
}

func TestMutateClaimSpec_ExplicitGateProfileWins(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	job := testMutatorJobWithRecoveryMeta(jobID, "{}")
	profile := []byte(`{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"echo repo","env":{}},"unit":{"status":"passed","command":"echo repo unit","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`)

	merged, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:            []byte(`{"build_gate":{"pre":{"gate_profile":{"command":"echo explicit","env":{"SRC":"explicit"}}}}}`),
		job:             job,
		jobType:         domaintypes.JobTypePreGate,
		repoGateProfile: profile,
	})
	if err != nil {
		t.Fatalf("mutateClaimSpec: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	bg := out["build_gate"].(map[string]any)
	pre := bg["pre"].(map[string]any)
	gp := pre["gate_profile"].(map[string]any)
	if got := gp["command"]; got != "echo explicit" {
		t.Fatalf("build_gate.pre.gate_profile.command=%v, want echo explicit", got)
	}
}

func TestMutateClaimSpec_PreservesPhaseTargetAndAlways(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	job := testMutatorJobWithRecoveryMeta(jobID, "{}")
	profile := []byte(`{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"echo repo","env":{}},"unit":{"status":"passed","command":"echo repo unit","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`)

	merged, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:            []byte(`{"build_gate":{"pre":{"target":"unit","always":true}}}`),
		job:             job,
		jobType:         domaintypes.JobTypePreGate,
		repoGateProfile: profile,
	})
	if err != nil {
		t.Fatalf("mutateClaimSpec: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	bg := out["build_gate"].(map[string]any)
	pre := bg["pre"].(map[string]any)
	if got := pre["target"]; got != contracts.GateProfileTargetUnit {
		t.Fatalf("build_gate.pre.target=%v, want %q", got, contracts.GateProfileTargetUnit)
	}
	if got := pre["always"]; got != true {
		t.Fatalf("build_gate.pre.always=%v, want true", got)
	}
	gp := pre["gate_profile"].(map[string]any)
	if got := gp["command"]; got != "echo repo" {
		t.Fatalf("build_gate.pre.gate_profile.command=%v, want echo repo", got)
	}
}

func TestMutateClaimSpec_HealInfraAddsSchemaAndArtifacts(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	job := testMutatorJobWithRecoveryMeta(jobID, `{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","expectations":{"artifacts":[{"path":"/out/a.json"},{"path":"/out/a.json"},{"path":"/out/b.json"}]}}}`)

	merged, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:    []byte(`{"artifact_paths":["/out/existing.json","/out/a.json"]}`),
		job:     job,
		jobType: domaintypes.JobTypeHeal,
	})
	if err != nil {
		t.Fatalf("mutateClaimSpec: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	bg := out["build_gate"].(map[string]any)
	healing := bg["healing"].(map[string]any)
	if got := healing["selected_error_kind"]; got != "infra" {
		t.Fatalf("build_gate.healing.selected_error_kind=%v, want infra", got)
	}
	env := out["env"].(map[string]any)
	schemaRaw, ok := env[contracts.GateProfileSchemaJSONEnv].(string)
	if !ok || schemaRaw == "" {
		t.Fatalf("expected %s in env", contracts.GateProfileSchemaJSONEnv)
	}
	if !json.Valid([]byte(schemaRaw)) {
		t.Fatalf("expected %s to contain valid json", contracts.GateProfileSchemaJSONEnv)
	}

	paths, ok := out["artifact_paths"].([]any)
	if !ok {
		t.Fatalf("artifact_paths=%T, want []any", out["artifact_paths"])
	}
	got := map[string]struct{}{}
	for _, v := range paths {
		s, _ := v.(string)
		got[s] = struct{}{}
	}
	for _, want := range []string{"/out/existing.json", "/out/a.json", "/out/b.json"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("artifact_paths missing %s: %#v", want, paths)
		}
	}
}

func TestMutateClaimSpec_InvalidSpec(t *testing.T) {
	t.Parallel()

	job := testMutatorJobWithRecoveryMeta(domaintypes.NewJobID(), "{}")
	_, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:    []byte(`[]`),
		job:     job,
		jobType: domaintypes.JobTypeMig,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "merge job_id into spec") {
		t.Fatalf("expected merge job_id wrapper in error, got %q", got)
	}
}

func testMutatorJobWithRecoveryMeta(jobID domaintypes.JobID, meta string) store.Job {
	return store.Job{ID: jobID, Meta: []byte(meta)}
}
