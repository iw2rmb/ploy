package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestJobStatusConstants verifies that job status constants have the expected
// string values as defined by the v1 API contract.
func TestJobStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		status   types.JobStatus
		expected string
	}{
		{"Success", types.JobStatusSuccess, "Success"},
		{"Fail", types.JobStatusFail, "Fail"},
		{"Error", types.JobStatusError, "Error"},
		{"Cancelled", types.JobStatusCancelled, "Cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.String(); got != tt.expected {
				t.Errorf("types.JobStatus%s.String() = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}

// TestDiffJobTypeConstants verifies that diff job_type constants have the expected
// string values for correct filtering and tagging behavior.
func TestDiffJobTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		jobType  types.DiffJobType
		expected string
	}{
		{"Mig", types.DiffJobTypeMig, "mig"},
		{"Healing", types.DiffJobTypeHealing, "healing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.jobType.String(); got != tt.expected {
				t.Errorf("types.DiffJobType%s.String() = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}
