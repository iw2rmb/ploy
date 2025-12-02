package store

import (
	"testing"
)

// TestConvertToJobStatus verifies that ConvertToJobStatus correctly maps
// various string representations to canonical store.JobStatus values.
func TestConvertToJobStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    JobStatus
		wantErr bool
	}{
		// Direct matches
		{name: "created", input: "created", want: JobStatusCreated, wantErr: false},
		{name: "pending", input: "pending", want: JobStatusPending, wantErr: false},
		{name: "running", input: "running", want: JobStatusRunning, wantErr: false},
		{name: "succeeded", input: "succeeded", want: JobStatusSucceeded, wantErr: false},
		{name: "failed", input: "failed", want: JobStatusFailed, wantErr: false},
		{name: "skipped", input: "skipped", want: JobStatusSkipped, wantErr: false},
		{name: "canceled", input: "canceled", want: JobStatusCanceled, wantErr: false},

		// Mods API compatibility mappings
		{name: "queued->created", input: "queued", want: JobStatusCreated, wantErr: false},
		{name: "cancelled (UK)", input: "cancelled", want: JobStatusCanceled, wantErr: false},
		{name: "cancelling", input: "cancelling", want: JobStatusCanceled, wantErr: false},

		// Error cases
		{name: "unknown", input: "unknown", want: "", wantErr: true},
		{name: "empty", input: "", want: "", wantErr: true},
		{name: "invalid", input: "invalid-status", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertToJobStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertToJobStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ConvertToJobStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestConvertToRunStatus verifies that ConvertToRunStatus correctly maps
// various string representations to canonical store.RunStatus values.
func TestConvertToRunStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    RunStatus
		wantErr bool
	}{
		// Direct matches
		{name: "queued", input: "queued", want: RunStatusQueued, wantErr: false},
		{name: "assigned", input: "assigned", want: RunStatusAssigned, wantErr: false},
		{name: "running", input: "running", want: RunStatusRunning, wantErr: false},
		{name: "succeeded", input: "succeeded", want: RunStatusSucceeded, wantErr: false},
		{name: "failed", input: "failed", want: RunStatusFailed, wantErr: false},
		{name: "canceled", input: "canceled", want: RunStatusCanceled, wantErr: false},

		// Mods API compatibility mappings
		{name: "pending->queued", input: "pending", want: RunStatusQueued, wantErr: false},
		{name: "cancelled (UK)", input: "cancelled", want: RunStatusCanceled, wantErr: false},
		{name: "cancelling", input: "cancelling", want: RunStatusCanceled, wantErr: false},

		// Error cases
		{name: "unknown", input: "unknown", want: "", wantErr: true},
		{name: "empty", input: "", want: "", wantErr: true},
		{name: "invalid", input: "invalid-status", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertToRunStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertToRunStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ConvertToRunStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestValidateJobStatus verifies that ValidateJobStatus correctly validates
// canonical store.JobStatus values.
func TestValidateJobStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    JobStatus
		wantErr bool
	}{
		{name: "created", input: "created", want: JobStatusCreated, wantErr: false},
		{name: "pending", input: "pending", want: JobStatusPending, wantErr: false},
		{name: "running", input: "running", want: JobStatusRunning, wantErr: false},
		{name: "succeeded", input: "succeeded", want: JobStatusSucceeded, wantErr: false},
		{name: "failed", input: "failed", want: JobStatusFailed, wantErr: false},
		{name: "skipped", input: "skipped", want: JobStatusSkipped, wantErr: false},
		{name: "canceled", input: "canceled", want: JobStatusCanceled, wantErr: false},

		// These should fail validation (non-canonical values)
		{name: "queued invalid", input: "queued", want: "", wantErr: true},
		{name: "cancelled invalid", input: "cancelled", want: "", wantErr: true},
		{name: "cancelling invalid", input: "cancelling", want: "", wantErr: true},
		{name: "unknown", input: "unknown", want: "", wantErr: true},
		{name: "empty", input: "", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidateJobStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJobStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateJobStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestValidateRunStatus verifies that ValidateRunStatus correctly validates
// canonical store.RunStatus values.
func TestValidateRunStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    RunStatus
		wantErr bool
	}{
		{name: "queued", input: "queued", want: RunStatusQueued, wantErr: false},
		{name: "assigned", input: "assigned", want: RunStatusAssigned, wantErr: false},
		{name: "running", input: "running", want: RunStatusRunning, wantErr: false},
		{name: "succeeded", input: "succeeded", want: RunStatusSucceeded, wantErr: false},
		{name: "failed", input: "failed", want: RunStatusFailed, wantErr: false},
		{name: "canceled", input: "canceled", want: RunStatusCanceled, wantErr: false},

		// These should fail validation (non-canonical values)
		{name: "pending invalid", input: "pending", want: "", wantErr: true},
		{name: "cancelled invalid", input: "cancelled", want: "", wantErr: true},
		{name: "cancelling invalid", input: "cancelling", want: "", wantErr: true},
		{name: "unknown", input: "unknown", want: "", wantErr: true},
		{name: "empty", input: "", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidateRunStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRunStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateRunStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestValidateBuildgateJobStatus verifies that ValidateBuildgateJobStatus correctly validates
// canonical store.BuildgateJobStatus values.
func TestValidateBuildgateJobStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    BuildgateJobStatus
		wantErr bool
	}{
		{name: "pending", input: "pending", want: BuildgateJobStatusPending, wantErr: false},
		{name: "claimed", input: "claimed", want: BuildgateJobStatusClaimed, wantErr: false},
		{name: "running", input: "running", want: BuildgateJobStatusRunning, wantErr: false},
		{name: "completed", input: "completed", want: BuildgateJobStatusCompleted, wantErr: false},
		{name: "failed", input: "failed", want: BuildgateJobStatusFailed, wantErr: false},

		// Invalid cases
		{name: "unknown", input: "unknown", want: "", wantErr: true},
		{name: "empty", input: "", want: "", wantErr: true},
		{name: "succeeded", input: "succeeded", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidateBuildgateJobStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBuildgateJobStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateBuildgateJobStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
