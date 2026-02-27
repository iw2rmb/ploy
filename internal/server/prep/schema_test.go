package prep

import (
	"strings"
	"testing"
)

func TestValidateProfileJSON(t *testing.T) {
	t.Parallel()

	valid := []byte(validProfileJSON("repo_123"))
	if err := validateProfileJSON(valid); err != nil {
		t.Fatalf("validateProfileJSON(valid) error = %v", err)
	}

	invalid := []byte(`{"schema_version":1,"repo_id":"repo_123"}`)
	err := validateProfileJSON(invalid)
	if err == nil {
		t.Fatal("validateProfileJSON(invalid) expected error")
	}
	if !strings.Contains(err.Error(), "prep schema validation failed") {
		t.Fatalf("validateProfileJSON(invalid) error = %v, want schema validation error", err)
	}
}

func TestValidateProfileJSON_SimpleRejectsOrchestrationSteps(t *testing.T) {
	t.Parallel()

	invalid := []byte(`{
  "schema_version": 1,
  "repo_id": "repo_123",
  "runner_mode": "simple",
  "targets": {
    "build": {"status": "passed", "command": "go test ./...", "env": {}, "failure_code": null},
    "unit": {"status": "not_attempted", "env": {}},
    "all_tests": {"status": "not_attempted", "env": {}}
  },
  "orchestration": {
    "pre": [{"id": "prep", "type": "docker_network", "args": {}}],
    "post": []
  },
  "tactics_used": ["go_default"],
  "attempts": [],
  "evidence": {"log_refs": ["inline://prep/test"], "diagnostics": []},
  "repro_check": {"status": "passed", "details": "ok"},
  "prompt_delta_suggestion": {"status": "none", "summary": "", "candidate_lines": []}
}`)
	err := validateProfileJSON(invalid)
	if err == nil {
		t.Fatal("validateProfileJSON(invalid simple orchestration) expected error")
	}
	if !strings.Contains(err.Error(), "prep schema validation failed") {
		t.Fatalf("validateProfileJSON(invalid simple orchestration) error = %v, want schema validation error", err)
	}
}
