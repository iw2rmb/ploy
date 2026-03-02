package contracts

import (
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestGateProfileParseAndMapToGate(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
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

	profile, err := ParseGateProfileJSON(raw)
	if err != nil {
		t.Fatalf("ParseGateProfileJSON: %v", err)
	}

	phase, override, err := GateProfileGateOverrideForJobType(profile, types.JobTypePreGate)
	if err != nil {
		t.Fatalf("GateProfileGateOverrideForJobType(pre_gate): %v", err)
	}
	if phase != BuildGateProfilePhasePre {
		t.Fatalf("phase=%q, want %q", phase, BuildGateProfilePhasePre)
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
	if got := override.Env[GateProfileDockerHostEnv]; got != "unix:///var/run/docker.sock" {
		t.Fatalf("pre env[%s]=%q, want %q", GateProfileDockerHostEnv, got, "unix:///var/run/docker.sock")
	}
	if got := override.Env[GateProfileDockerAPIVersionEnv]; got != "1.45" {
		t.Fatalf("pre env[%s]=%q, want %q", GateProfileDockerAPIVersionEnv, got, "1.45")
	}

	phase, override, err = GateProfileGateOverrideForJobType(profile, types.JobTypePostGate)
	if err != nil {
		t.Fatalf("GateProfileGateOverrideForJobType(post_gate): %v", err)
	}
	if phase != BuildGateProfilePhasePost {
		t.Fatalf("phase=%q, want %q", phase, BuildGateProfilePhasePost)
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
	if got := override.Env[GateProfileDockerHostEnv]; got != "unix:///var/run/docker.sock" {
		t.Fatalf("post env[%s]=%q, want %q", GateProfileDockerHostEnv, got, "unix:///var/run/docker.sock")
	}

	phase, override, err = GateProfileGateOverrideForJobType(profile, types.JobTypeReGate)
	if err != nil {
		t.Fatalf("GateProfileGateOverrideForJobType(re_gate): %v", err)
	}
	if phase != BuildGateProfilePhasePost {
		t.Fatalf("phase=%q, want %q", phase, BuildGateProfilePhasePost)
	}
	if override == nil || override.Command.Shell != "go test ./... -run TestUnit" {
		t.Fatalf("re_gate override=%v, want unit command override", override)
	}

	phase, override, err = GateProfileGateOverrideForJobType(profile, types.JobTypeMod)
	if err != nil {
		t.Fatalf("GateProfileGateOverrideForJobType(mig): %v", err)
	}
	if phase != "" || override != nil {
		t.Fatalf("mig mapping = (%q, %v), want empty/nil", phase, override)
	}
}

func TestGateProfileParseRejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     []byte
		wantErr string
	}{
		{
			name:    "empty",
			raw:     nil,
			wantErr: "gate_profile: required",
		},
		{
			name:    "missing schema version",
			raw:     []byte(`{"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.schema_version",
		},
		{
			name:    "invalid target status",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"build":{"status":"bad","env":{}},"unit":{"status":"passed","command":"go test ./...","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.targets.build.status",
		},
		{
			name:    "passed missing command",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"build":{"status":"passed","env":{}},"unit":{"status":"passed","command":"go test ./...","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.targets.build.command",
		},
		{
			name:    "simple mode rejects orchestration steps",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[{"id":"x"}],"post":[]}}`),
			wantErr: "simple mode must not define pre/post steps",
		},
		{
			name:    "runtime tcp requires host",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"runtime":{"docker":{"mode":"tcp"}},"targets":{"build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.runtime.docker.host",
		},
		{
			name:    "runtime host forbidden for host_socket",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"runtime":{"docker":{"mode":"host_socket","host":"tcp://docker:2375"}},"targets":{"build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.runtime.docker.host: forbidden",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseGateProfileJSON(tc.raw)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if got := err.Error(); got == "" || !strings.Contains(got, tc.wantErr) {
				t.Fatalf("error=%q, want substring %q", got, tc.wantErr)
			}
		})
	}
}

func TestGateProfileMapToGateUsesCommandsRegardlessOfTargetStatus(t *testing.T) {
	t.Parallel()

	profile, err := ParseGateProfileJSON([]byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"targets": {
			"build": {"status":"not_attempted","command":"go test ./...","env":{}},
			"unit": {"status":"failed","command":"go test ./... -run TestUnit","env":{},"failure_code":"unknown"},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []}
	}`))
	if err != nil {
		t.Fatalf("ParseGateProfileJSON: %v", err)
	}

	_, preOverride, err := GateProfileGateOverrideForJobType(profile, types.JobTypePreGate)
	if err != nil {
		t.Fatalf("GateProfileGateOverrideForJobType(pre_gate): %v", err)
	}
	if preOverride == nil {
		t.Fatal("pre override=nil, want non-nil")
	}
	if got, want := preOverride.Command.Shell, "go test ./..."; got != want {
		t.Fatalf("pre command=%q, want %q", got, want)
	}

	_, postOverride, err := GateProfileGateOverrideForJobType(profile, types.JobTypePostGate)
	if err != nil {
		t.Fatalf("GateProfileGateOverrideForJobType(post_gate): %v", err)
	}
	if postOverride == nil {
		t.Fatal("post override=nil, want non-nil")
	}
	if got, want := postOverride.Command.Shell, "go test ./... -run TestUnit"; got != want {
		t.Fatalf("post command=%q, want %q", got, want)
	}
}

func TestGateProfileMapToGateSkipsWhenMappedTargetHasNoCommand(t *testing.T) {
	t.Parallel()

	profile, err := ParseGateProfileJSON([]byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"targets": {
			"build": {"status":"not_attempted","env":{}},
			"unit": {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []}
	}`))
	if err != nil {
		t.Fatalf("ParseGateProfileJSON: %v", err)
	}

	_, preOverride, err := GateProfileGateOverrideForJobType(profile, types.JobTypePreGate)
	if err != nil {
		t.Fatalf("GateProfileGateOverrideForJobType(pre_gate): %v", err)
	}
	if preOverride != nil {
		t.Fatalf("pre override=%v, want nil", preOverride)
	}

	_, reOverride, err := GateProfileGateOverrideForJobType(profile, types.JobTypeReGate)
	if err != nil {
		t.Fatalf("GateProfileGateOverrideForJobType(re_gate): %v", err)
	}
	if reOverride != nil {
		t.Fatalf("re_gate override=%v, want nil", reOverride)
	}
}

func TestGateProfileRuntimeTCPMapsToGateEnv(t *testing.T) {
	t.Parallel()

	profile, err := ParseGateProfileJSON([]byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
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
		t.Fatalf("ParseGateProfileJSON: %v", err)
	}

	_, override, err := GateProfileGateOverrideForJobType(profile, types.JobTypePreGate)
	if err != nil {
		t.Fatalf("GateProfileGateOverrideForJobType(pre_gate): %v", err)
	}
	if override == nil {
		t.Fatal("override=nil, want non-nil")
	}
	if got := override.Env[GateProfileDockerHostEnv]; got != "tcp://prep-dind:2375" {
		t.Fatalf("env[%s]=%q, want %q", GateProfileDockerHostEnv, got, "tcp://prep-dind:2375")
	}
}

func TestGateProfileStackMatches_ReleaseSemantics(t *testing.T) {
	t.Parallel()

	profile, err := ParseGateProfileJSON([]byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"java","tool":"gradle","release":"11"},
		"targets": {
			"build": {"status":"passed","command":"./gradlew test","env":{},"failure_code":null},
			"unit": {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []}
	}`))
	if err != nil {
		t.Fatalf("ParseGateProfileJSON: %v", err)
	}

	if !GateProfileStackMatches(profile, "java", "gradle", "") {
		t.Fatal("expected empty expected release to act as wildcard")
	}
	if !GateProfileStackMatches(profile, "java", "gradle", "11") {
		t.Fatal("expected exact release match to pass")
	}
	if GateProfileStackMatches(profile, "java", "gradle", "17") {
		t.Fatal("expected mismatched release to fail")
	}
}
