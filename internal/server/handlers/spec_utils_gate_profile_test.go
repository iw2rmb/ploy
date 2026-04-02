package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestApplyRepoGateProfileMutator(t *testing.T) {
	t.Parallel()

	profile := []byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"runtime": {
			"docker": {
				"mode": "host_socket"
			}
		},
		"targets": {
			"active": "unit",
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
		},
		"orchestration": {
			"pre": [],
			"post": []
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
			name:      "injects pre_gate from active target",
			jobType:   domaintypes.JobTypePreGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}]}`),
			profile:   profile,
			wantPhase: "pre",
			wantCmd:   "go test ./... -run TestUnit",
			wantEnvK:  "CGO_ENABLED",
			wantEnvV:  "0",
		},
		{
			name:      "injects post_gate from active target",
			jobType:   domaintypes.JobTypePostGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}]}`),
			profile:   profile,
			wantPhase: "post",
			wantCmd:   "go test ./... -run TestUnit",
			wantEnvK:  "CGO_ENABLED",
			wantEnvV:  "0",
		},
		{
			name:      "injects re_gate from active target",
			jobType:   domaintypes.JobTypeReGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}]}`),
			profile:   profile,
			wantPhase: "post",
			wantCmd:   "go test ./... -run TestUnit",
			wantEnvK:  "CGO_ENABLED",
			wantEnvV:  "0",
		},
		{
			name:    "explicit spec gate_profile wins over profile mapping",
			jobType: domaintypes.JobTypePreGate,
			spec: []byte(`{
				"steps":[{"image":"docker.io/acme/mig:latest"}],
				"build_gate":{"pre":{"gate_profile":{"command":"echo explicit","env":{"X":"1"}}}}
			}`),
			profile:   profile,
			wantPhase: "pre",
			wantCmd:   "echo explicit",
			wantEnvK:  "X",
			wantEnvV:  "1",
		},
		{
			name:    "non-gate job does not inject",
			jobType: domaintypes.JobTypeMig,
			spec:    []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}]}`),
			profile: profile,
		},
		{
			name:      "maps even when target status is failed",
			jobType:   domaintypes.JobTypePreGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}]}`),
			wantPhase: "pre",
			wantCmd:   "go test ./...",
			profile: []byte(`{
				"schema_version": 1,
				"repo_id": "repo_123",
				"runner_mode": "simple",
				"stack": {"language":"go","tool":"go"},
				"targets": {
					"active": "build",
					"build": {"status": "failed", "command":"go test ./...", "env": {}, "failure_code": "unknown"},
					"unit": {"status": "passed", "command":"go test ./... -run TestUnit", "env": {}, "failure_code": null},
					"all_tests": {"status": "not_attempted", "env": {}}
				},
				"orchestration": {"pre": [], "post": []}
			}`),
		},
		{
			name:    "unsupported active target is terminal and does not inject",
			jobType: domaintypes.JobTypePostGate,
			spec:    []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}]}`),
			profile: []byte(`{
				"schema_version": 1,
				"repo_id": "repo_123",
				"runner_mode": "simple",
				"stack": {"language":"go","tool":"go"},
				"targets": {
					"active": "unsupported",
					"build": {"status":"failed","command":"go test ./...","env":{},"failure_code":"infra_support"},
					"unit": {"status":"not_attempted","env":{}},
					"all_tests": {"status":"not_attempted","env":{}}
				},
				"orchestration": {"pre": [], "post": []}
			}`),
		},
		{
			name:    "invalid profile returns error",
			jobType: domaintypes.JobTypePreGate,
			spec:    []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}]}`),
			profile: []byte(`{"schema_version":1}`),
			wantErr: "gate_profile",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, err := parseSpecObjectStrict(json.RawMessage(tc.spec))
			if err != nil {
				t.Fatalf("parseSpecObjectStrict: %v", err)
			}

			err = applyRepoGateProfileMutator(m, tc.profile, tc.jobType)
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
				t.Fatalf("applyRepoGateProfileMutator: %v", err)
			}

			if tc.wantPhase == "" {
				if _, ok := m["build_gate"]; ok {
					t.Fatalf("build_gate unexpectedly present: %v", m["build_gate"])
				}
				return
			}

			bg, ok := m["build_gate"].(map[string]any)
			if !ok {
				t.Fatalf("build_gate missing or invalid: %T", m["build_gate"])
			}
			phaseObj, ok := bg[tc.wantPhase].(map[string]any)
			if !ok {
				t.Fatalf("build_gate.%s missing or invalid: %T", tc.wantPhase, bg[tc.wantPhase])
			}
			prepObj, ok := phaseObj["gate_profile"].(map[string]any)
			if !ok {
				t.Fatalf("build_gate.%s.gate_profile missing or invalid: %T", tc.wantPhase, phaseObj["gate_profile"])
			}

			if got := prepObj["command"]; got != tc.wantCmd {
				t.Fatalf("build_gate.%s.gate_profile.command=%v, want %q", tc.wantPhase, got, tc.wantCmd)
			}
			if tc.wantEnvK != "" {
				env, ok := prepObj["env"].(map[string]any)
				if !ok {
					t.Fatalf("build_gate.%s.gate_profile.env missing or invalid: %T", tc.wantPhase, prepObj["env"])
				}
				if got := env[tc.wantEnvK]; got != tc.wantEnvV {
					t.Fatalf("build_gate.%s.gate_profile.env[%s]=%v, want %q", tc.wantPhase, tc.wantEnvK, got, tc.wantEnvV)
				}
			}
		})
	}
}
