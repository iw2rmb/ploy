package contracts

import (
	"strings"
	"testing"
)

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
			raw:     []byte(`{"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build"},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.schema_version",
		},
		{
			name:    "missing active target",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.targets.active: required",
		},
		{
			name:    "invalid active target",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"mod","build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.targets.active: invalid value",
		},
		{
			name:    "invalid target status",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"bad","env":{}},"unit":{"status":"passed","command":"go test ./...","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.targets.build.status",
		},
		{
			name:    "passed missing command",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","env":{}},"unit":{"status":"passed","command":"go test ./...","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.targets.build.command",
		},
		{
			name:    "active runnable target requires command",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"all_tests","build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"passed","command":"go test ./... -run Unit","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.targets.all_tests.command",
		},
		{
			name:    "simple mode rejects orchestration steps",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[{"id":"x"}],"post":[]}}`),
			wantErr: "simple mode must not define pre/post steps",
		},
		{
			name:    "runtime tcp requires host",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"runtime":{"docker":{"mode":"tcp"}},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.runtime.docker.host",
		},
		{
			name:    "runtime host forbidden for host_socket",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"runtime":{"docker":{"mode":"host_socket","host":"tcp://docker:2375"}},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.runtime.docker.host: forbidden",
		},
		{
			name:    "unsupported requires failed build status",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"unsupported","build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.targets.build.status",
		},
		{
			name:    "unsupported requires infra_support failure code",
			raw:     []byte(`{"schema_version":1,"repo_id":"repo_123","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"unsupported","build":{"status":"failed","command":"go test ./...","env":{},"failure_code":"unknown"},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`),
			wantErr: "gate_profile.targets.build.failure_code",
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
