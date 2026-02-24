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

// TestDiffJobTypeConstants verifies that diff job_type constants have the expected
// string values for correct filtering and tagging behavior.
func TestDiffJobTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		jobType  DiffJobType
		expected string
	}{
		{"Mod", DiffJobTypeMod, "mod"},
		{"Healing", DiffJobTypeHealing, "healing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.jobType.String(); got != tt.expected {
				t.Errorf("DiffJobType%s.String() = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}
