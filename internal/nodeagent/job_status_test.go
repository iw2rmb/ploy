package nodeagent

import "testing"

// TestJobStatusConstants verifies that job status constants have the expected
// string values as defined by the v1 API contract.
func TestJobStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		status   JobStatus
		expected string
	}{
		{"Success", JobStatusSuccess, "Success"},
		{"Fail", JobStatusFail, "Fail"},
		{"Cancelled", JobStatusCancelled, "Cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.String(); got != tt.expected {
				t.Errorf("JobStatus%s.String() = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}

// TestDiffModTypeConstants verifies that diff mod_type constants have the expected
// string values for correct filtering and tagging behavior.
func TestDiffModTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		modType  DiffModType
		expected string
	}{
		{"Mod", DiffModTypeMod, "mod"},
		{"Healing", DiffModTypeHealing, "healing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.modType.String(); got != tt.expected {
				t.Errorf("DiffModType%s.String() = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}
