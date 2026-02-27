package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestMergeRepoPrepProfileIntoSpec(t *testing.T) {
	t.Parallel()

	profile := []byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"targets": {
			"build": {
				"status": "passed",
				"command": "go test ./...",
				"env": {"GOFLAGS":"-mod=readonly"},
				"failure_code": null
			},
			"unit": {
				"status": "passed",
				"command": "go test ./... -run TestUnit",
				"env": {"CGO_ENABLED":"0"},
				"failure_code": null
			},
			"all_tests": {
				"status": "not_attempted",
				"env": {}
			}
		}
	}`)

	tests := []struct {
		name      string
		jobType   domaintypes.JobType
		spec      []byte
		profile   []byte
		wantPhase string
		wantCmd   string
		wantEnvK  string
		wantEnvV  string
		wantErr   string
	}{
		{
			name:      "injects pre_gate from build target",
			jobType:   domaintypes.JobTypePreGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			profile:   profile,
			wantPhase: "pre",
			wantCmd:   "go test ./...",
			wantEnvK:  "GOFLAGS",
			wantEnvV:  "-mod=readonly",
		},
		{
			name:      "injects post_gate from unit target",
			jobType:   domaintypes.JobTypePostGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			profile:   profile,
			wantPhase: "post",
			wantCmd:   "go test ./... -run TestUnit",
			wantEnvK:  "CGO_ENABLED",
			wantEnvV:  "0",
		},
		{
			name:      "injects re_gate from unit target",
			jobType:   domaintypes.JobTypeReGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			profile:   profile,
			wantPhase: "post",
			wantCmd:   "go test ./... -run TestUnit",
			wantEnvK:  "CGO_ENABLED",
			wantEnvV:  "0",
		},
		{
			name:    "explicit spec prep wins over profile mapping",
			jobType: domaintypes.JobTypePreGate,
			spec: []byte(`{
				"steps":[{"image":"docker.io/acme/mod:latest"}],
				"build_gate":{"pre":{"prep":{"command":"echo explicit","env":{"X":"1"}}}}
			}`),
			profile:   profile,
			wantPhase: "pre",
			wantCmd:   "echo explicit",
			wantEnvK:  "X",
			wantEnvV:  "1",
		},
		{
			name:    "non-gate job does not inject",
			jobType: domaintypes.JobTypeMod,
			spec:    []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			profile: profile,
		},
		{
			name:    "skip mapping when target not passed",
			jobType: domaintypes.JobTypePreGate,
			spec:    []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			profile: []byte(`{
				"schema_version": 1,
				"repo_id": "repo_123",
				"runner_mode": "simple",
				"targets": {
					"build": {"status": "failed", "command":"go test ./...", "env": {}, "failure_code": "unknown"},
					"unit": {"status": "passed", "command":"go test ./... -run TestUnit", "env": {}, "failure_code": null},
					"all_tests": {"status": "not_attempted", "env": {}}
				}
			}`),
		},
		{
			name:    "invalid profile returns error",
			jobType: domaintypes.JobTypePreGate,
			spec:    []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			profile: []byte(`{"schema_version":1}`),
			wantErr: "prep_profile",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			merged, err := mergeRepoPrepProfileIntoSpec(tc.spec, tc.profile, tc.jobType)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if got := err.Error(); !strings.Contains(got, tc.wantErr) {
					t.Fatalf("error=%q, want substring %q", got, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("mergeRepoPrepProfileIntoSpec: %v", err)
			}

			var specMap map[string]any
			if err := json.Unmarshal(merged, &specMap); err != nil {
				t.Fatalf("unmarshal merged spec: %v", err)
			}

			if tc.wantPhase == "" {
				if _, ok := specMap["build_gate"]; ok {
					t.Fatalf("build_gate unexpectedly present: %v", specMap["build_gate"])
				}
				return
			}

			bg, ok := specMap["build_gate"].(map[string]any)
			if !ok {
				t.Fatalf("build_gate missing or invalid: %T", specMap["build_gate"])
			}
			phaseObj, ok := bg[tc.wantPhase].(map[string]any)
			if !ok {
				t.Fatalf("build_gate.%s missing or invalid: %T", tc.wantPhase, bg[tc.wantPhase])
			}
			prepObj, ok := phaseObj["prep"].(map[string]any)
			if !ok {
				t.Fatalf("build_gate.%s.prep missing or invalid: %T", tc.wantPhase, phaseObj["prep"])
			}

			if got := prepObj["command"]; got != tc.wantCmd {
				t.Fatalf("build_gate.%s.prep.command=%v, want %q", tc.wantPhase, got, tc.wantCmd)
			}
			if tc.wantEnvK != "" {
				env, ok := prepObj["env"].(map[string]any)
				if !ok {
					t.Fatalf("build_gate.%s.prep.env missing or invalid: %T", tc.wantPhase, prepObj["env"])
				}
				if got := env[tc.wantEnvK]; got != tc.wantEnvV {
					t.Fatalf("build_gate.%s.prep.env[%s]=%v, want %q", tc.wantPhase, tc.wantEnvK, got, tc.wantEnvV)
				}
			}
		})
	}
}
