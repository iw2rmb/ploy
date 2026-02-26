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
