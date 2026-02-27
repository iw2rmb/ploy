package contracts

import (
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestPrepProfileParseAndMapToGate(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"runtime": {
			"docker": {
				"mode": "host_socket",
				"api_version": "1.45"
			}
		},
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
		},
		"orchestration": {
			"pre": [],
			"post": []
		}
	}`)

	profile, err := ParsePrepProfileJSON(raw)
	if err != nil {
		t.Fatalf("ParsePrepProfileJSON: %v", err)
	}

	phase, override, err := PrepProfileGateOverrideForJobType(profile, types.JobTypePreGate)
	if err != nil {
		t.Fatalf("PrepProfileGateOverrideForJobType(pre_gate): %v", err)
	}
	if phase != BuildGatePrepPhasePre {
		t.Fatalf("phase=%q, want %q", phase, BuildGatePrepPhasePre)
	}
	if override == nil {
		t.Fatal("override=nil, want non-nil")
	}
	if override.Command.Shell != "go test ./..." {
		t.Fatalf("pre command=%q, want %q", override.Command.Shell, "go test ./...")
	}
	if got := override.Env["GOFLAGS"]; got != "-mod=readonly" {
		t.Fatalf("pre env[GOFLAGS]=%q, want %q", got, "-mod=readonly")
	}
	if got := override.Env[PrepDockerHostEnv]; got != "unix:///var/run/docker.sock" {
		t.Fatalf("pre env[%s]=%q, want %q", PrepDockerHostEnv, got, "unix:///var/run/docker.sock")
	}
	if got := override.Env[PrepDockerAPIVersionEnv]; got != "1.45" {
		t.Fatalf("pre env[%s]=%q, want %q", PrepDockerAPIVersionEnv, got, "1.45")
	}

	phase, override, err = PrepProfileGateOverrideForJobType(profile, types.JobTypePostGate)
	if err != nil {
		t.Fatalf("PrepProfileGateOverrideForJobType(post_gate): %v", err)
	}
	if phase != BuildGatePrepPhasePost {
		t.Fatalf("phase=%q, want %q", phase, BuildGatePrepPhasePost)
	}
	if override == nil {
		t.Fatal("post override=nil, want non-nil")
	}
	if override.Command.Shell != "go test ./... -run TestUnit" {
		t.Fatalf("post command=%q, want %q", override.Command.Shell, "go test ./... -run TestUnit")
	}
	if got := override.Env["CGO_ENABLED"]; got != "0" {
		t.Fatalf("post env[CGO_ENABLED]=%q, want %q", got, "0")
	}
	if got := override.Env[PrepDockerHostEnv]; got != "unix:///var/run/docker.sock" {
		t.Fatalf("post env[%s]=%q, want %q", PrepDockerHostEnv, got, "unix:///var/run/docker.sock")
	}

	phase, override, err = PrepProfileGateOverrideForJobType(profile, types.JobTypeReGate)
	if err != nil {
		t.Fatalf("PrepProfileGateOverrideForJobType(re_gate): %v", err)
	}
	if phase != BuildGatePrepPhasePost {
		t.Fatalf("phase=%q, want %q", phase, BuildGatePrepPhasePost)
	}
	if override == nil || override.Command.Shell != "go test ./... -run TestUnit" {
		t.Fatalf("re_gate override=%v, want unit command override", override)
	}

	phase, override, err = PrepProfileGateOverrideForJobType(profile, types.JobTypeMod)
	if err != nil {
		t.Fatalf("PrepProfileGateOverrideForJobType(mig): %v", err)
	}
	if phase != "" || override != nil {
		t.Fatalf("mig mapping = (%q, %v), want empty/nil", phase, override)
	}
}

func TestPrepProfileParseRejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     []byte
		wantErr string
	}{
		{
			name:    "empty",
			raw:     nil,
			wantErr: "prep_profile: required",
		},
		{
			name:    "missing schema version",
			raw:     []byte(`{"repo_id":"repo_123","runner_mode":"simple","targets":{},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "prep_profile.schema_version",
		},
		{
			name:    "invalid target status",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","targets":{"build":{"status":"bad","env":{}},"unit":{"status":"passed","command":"go test ./...","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "prep_profile.targets.build.status",
		},
		{
			name:    "passed missing command",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","targets":{"build":{"status":"passed","env":{}},"unit":{"status":"passed","command":"go test ./...","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "prep_profile.targets.build.command",
		},
		{
			name:    "simple mode rejects orchestration steps",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","targets":{"build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[{"id":"x"}],"post":[]}}`),
			wantErr: "simple mode must not define pre/post steps",
		},
		{
			name:    "runtime tcp requires host",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","runtime":{"docker":{"mode":"tcp"}},"targets":{"build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "prep_profile.runtime.docker.host",
		},
		{
			name:    "runtime host forbidden for host_socket",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","runtime":{"docker":{"mode":"host_socket","host":"tcp://docker:2375"}},"targets":{"build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "prep_profile.runtime.docker.host: forbidden",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePrepProfileJSON(tc.raw)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if got := err.Error(); got == "" || !strings.Contains(got, tc.wantErr) {
				t.Fatalf("error=%q, want substring %q", got, tc.wantErr)
			}
		})
	}
}

func TestPrepProfileMapToGateSkipsWhenNotPassedOrMissingCommand(t *testing.T) {
	t.Parallel()

	profile, err := ParsePrepProfileJSON([]byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"targets": {
			"build": {"status":"not_attempted","env":{}},
			"unit": {"status":"failed","command":"go test ./... -run TestUnit","env":{},"failure_code":"unknown"},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []}
	}`))
	if err != nil {
		t.Fatalf("ParsePrepProfileJSON: %v", err)
	}

	_, preOverride, err := PrepProfileGateOverrideForJobType(profile, types.JobTypePreGate)
	if err != nil {
		t.Fatalf("PrepProfileGateOverrideForJobType(pre_gate): %v", err)
	}
	if preOverride != nil {
		t.Fatalf("pre override=%v, want nil", preOverride)
	}

	_, postOverride, err := PrepProfileGateOverrideForJobType(profile, types.JobTypePostGate)
	if err != nil {
		t.Fatalf("PrepProfileGateOverrideForJobType(post_gate): %v", err)
	}
	if postOverride != nil {
		t.Fatalf("post override=%v, want nil", postOverride)
	}
}

func TestPrepProfileRuntimeTCPMapsToGateEnv(t *testing.T) {
	t.Parallel()

	profile, err := ParsePrepProfileJSON([]byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"runtime": {
			"docker": {
				"mode": "tcp",
				"host": "tcp://prep-dind:2375"
			}
		},
		"targets": {
			"build": {"status":"passed","command":"go test ./...","env":{},"failure_code":null},
			"unit": {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []}
	}`))
	if err != nil {
		t.Fatalf("ParsePrepProfileJSON: %v", err)
	}

	_, override, err := PrepProfileGateOverrideForJobType(profile, types.JobTypePreGate)
	if err != nil {
		t.Fatalf("PrepProfileGateOverrideForJobType(pre_gate): %v", err)
	}
	if override == nil {
		t.Fatal("override=nil, want non-nil")
	}
	if got := override.Env[PrepDockerHostEnv]; got != "tcp://prep-dind:2375" {
		t.Fatalf("env[%s]=%q, want %q", PrepDockerHostEnv, got, "tcp://prep-dind:2375")
	}
}
