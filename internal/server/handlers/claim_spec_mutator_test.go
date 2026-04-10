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

// mustMutateAndUnmarshal calls mutateClaimSpec and unmarshals the result.
func mustMutateAndUnmarshal(t *testing.T, input claimSpecMutatorInput) map[string]any {
	t.Helper()
	merged, err := mutateClaimSpec(input)
	if err != nil {
		t.Fatalf("mutateClaimSpec: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	return out
}

func TestMutateClaimSpec_GateProfileResolution(t *testing.T) {
	t.Parallel()

	repoProfile := []byte(`{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"echo repo","env":{}},"unit":{"status":"passed","command":"echo repo unit","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`)

	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name            string
		spec            []byte
		jobType         domaintypes.JobType
		recoveryMeta    string
		repoGateProfile []byte
		gitLab          config.GitLabConfig
		globalEnv       map[string][]GlobalEnvVar
		// assertions on build_gate.<phase>
		phase       string // "pre" or "post"
		wantGateCmd string
		wantGateEnv map[string]string
		wantTarget  string
		wantAlways  *bool
		// assertions on top-level fields
		wantEnvs map[string]string
		checkPAT bool
	}{
		{
			name:    "candidate profile wins over repo profile for re_gate",
			spec:    []byte(`{"envs":{"EXISTING":"1"}}`),
			jobType: domaintypes.JobTypeReGate,
			recoveryMeta: fmt.Sprintf(
				`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_validation_status":"%s","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"unit","unit":{"status":"passed","command":"echo candidate","env":{"SRC":"candidate"}},"build":{"status":"passed","command":"echo candidate build","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}}}`,
				contracts.RecoveryCandidateStatusValid,
			),
			repoGateProfile: []byte(`{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"unit","unit":{"status":"passed","command":"echo repo","env":{"SRC":"repo"}},"build":{"status":"passed","command":"echo repo build","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			gitLab:          config.GitLabConfig{Token: "server-token", Domain: "https://gitlab.example.com"},
			globalEnv:       map[string][]GlobalEnvVar{"GLOBAL": {{Value: "g", Target: domaintypes.GlobalEnvTargetGates}}},
			phase:           "post",
			wantGateCmd:     "echo candidate",
			wantGateEnv:     map[string]string{"SRC": "candidate"},
			wantEnvs:        map[string]string{"EXISTING": "1", "GLOBAL": "g"},
			checkPAT:        true,
		},
		{
			name:            "explicit spec gate_profile wins over repo profile",
			spec:            []byte(`{"build_gate":{"pre":{"gate_profile":{"command":"echo explicit","env":{"SRC":"explicit"}}}}}`),
			jobType:         domaintypes.JobTypePreGate,
			recoveryMeta:    "{}",
			repoGateProfile: repoProfile,
			phase:           "pre",
			wantGateCmd:     "echo explicit",
		},
		{
			name:            "preserves phase target and always, fills gate_profile from repo",
			spec:            []byte(`{"build_gate":{"pre":{"target":"unit","always":true}}}`),
			jobType:         domaintypes.JobTypePreGate,
			recoveryMeta:    "{}",
			repoGateProfile: repoProfile,
			phase:           "pre",
			wantGateCmd:     "echo repo",
			wantTarget:      contracts.GateProfileTargetUnit,
			wantAlways:      boolPtr(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			jobID := domaintypes.NewJobID()
			out := mustMutateAndUnmarshal(t, claimSpecMutatorInput{
				spec:            tt.spec,
				job:             store.Job{ID: jobID, Meta: []byte(tt.recoveryMeta)},
				jobType:         tt.jobType,
				gitLab:          tt.gitLab,
				globalEnv:       tt.globalEnv,
				repoGateProfile: tt.repoGateProfile,
			})

			if got := out["job_id"]; got != jobID.String() {
				t.Fatalf("job_id=%v, want %s", got, jobID.String())
			}
			if tt.checkPAT {
				if got := out["gitlab_pat"]; got != tt.gitLab.Token {
					t.Fatalf("gitlab_pat=%v, want %s", got, tt.gitLab.Token)
				}
				if got := out["gitlab_domain"]; got != tt.gitLab.Domain {
					t.Fatalf("gitlab_domain=%v, want %s", got, tt.gitLab.Domain)
				}
			}
			for k, want := range tt.wantEnvs {
				envs := out["envs"].(map[string]any)
				if got := envs[k]; got != want {
					t.Fatalf("envs[%s]=%v, want %s", k, got, want)
				}
			}

			bg := out["build_gate"].(map[string]any)
			phase := bg[tt.phase].(map[string]any)

			if tt.wantTarget != "" {
				if got := phase["target"]; got != tt.wantTarget {
					t.Fatalf("build_gate.%s.target=%v, want %q", tt.phase, got, tt.wantTarget)
				}
			}
			if tt.wantAlways != nil {
				if got := phase["always"]; got != *tt.wantAlways {
					t.Fatalf("build_gate.%s.always=%v, want %v", tt.phase, got, *tt.wantAlways)
				}
			}

			gp := phase["gate_profile"].(map[string]any)
			if tt.wantGateCmd != "" {
				if got := gp["command"]; got != tt.wantGateCmd {
					t.Fatalf("build_gate.%s.gate_profile.command=%v, want %s", tt.phase, got, tt.wantGateCmd)
				}
			}
			for k, want := range tt.wantGateEnv {
				gpEnv := gp["env"].(map[string]any)
				if got := gpEnv[k]; got != want {
					t.Fatalf("build_gate.%s.gate_profile.env[%s]=%v, want %s", tt.phase, k, got, want)
				}
			}
		})
	}
}

func TestMutateClaimSpec_HealInfraAddsSchemaAndArtifacts(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	out := mustMutateAndUnmarshal(t, claimSpecMutatorInput{
		spec:    []byte(`{"artifact_paths":["/out/existing.json","/out/a.json"]}`),
		job:     store.Job{ID: jobID, Meta: []byte(`{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","expectations":{"artifacts":[{"path":"/out/a.json"},{"path":"/out/a.json"},{"path":"/out/b.json"}]}}}`)},
		jobType: domaintypes.JobTypeHeal,
	})

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

	_, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:    []byte(`[]`),
		job:     store.Job{ID: domaintypes.NewJobID(), Meta: []byte(`{}`)},
		jobType: domaintypes.JobTypeMig,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "merge job_id into spec") {
		t.Fatalf("expected merge job_id wrapper in error, got %q", got)
	}
}
