package store

import (
	"testing"
)

// TestConvertToJobStatus verifies that ConvertToJobStatus correctly maps
// v1 canonical strings to store.JobStatus values.
// v1 status model: Created, Queued, Running, Success, Fail, Cancelled.
// The "skipped" status was removed in v1.
func TestConvertToJobStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    JobStatus
		wantErr bool
	}{
		// v1 canonical values (capitalized)
		{name: "Created", input: "Created", want: JobStatusCreated, wantErr: false},
		{name: "Queued", input: "Queued", want: JobStatusQueued, wantErr: false},
		{name: "Running", input: "Running", want: JobStatusRunning, wantErr: false},
		{name: "Success", input: "Success", want: JobStatusSuccess, wantErr: false},
		{name: "Fail", input: "Fail", want: JobStatusFail, wantErr: false},
		{name: "Cancelled", input: "Cancelled", want: JobStatusCancelled, wantErr: false},

		// v0 lowercase values should be rejected in v1
		{name: "v0 created rejected", input: "created", want: "", wantErr: true},
		{name: "v0 pending rejected", input: "pending", want: "", wantErr: true},
		{name: "v0 running rejected", input: "running", want: "", wantErr: true},
		{name: "v0 succeeded rejected", input: "succeeded", want: "", wantErr: true},
		{name: "v0 failed rejected", input: "failed", want: "", wantErr: true},
		{name: "v0 skipped rejected", input: "skipped", want: "", wantErr: true},
		{name: "v0 canceled rejected", input: "canceled", want: "", wantErr: true},

		// Other invalid inputs
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
// v1 canonical strings to store.RunStatus values.
// v1 status model: Started, Cancelled, Finished.
// The "queued", "assigned", "running", "succeeded", "failed" statuses were removed in v1.
func TestConvertToRunStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    RunStatus
		wantErr bool
	}{
		// v1 canonical values (capitalized)
		{name: "Started", input: "Started", want: RunStatusStarted, wantErr: false},
		{name: "Cancelled", input: "Cancelled", want: RunStatusCancelled, wantErr: false},
		{name: "Finished", input: "Finished", want: RunStatusFinished, wantErr: false},

		// v0 lowercase values should be rejected in v1
		{name: "v0 queued rejected", input: "queued", want: "", wantErr: true},
		{name: "v0 assigned rejected", input: "assigned", want: "", wantErr: true},
		{name: "v0 running rejected", input: "running", want: "", wantErr: true},
		{name: "v0 succeeded rejected", input: "succeeded", want: "", wantErr: true},
		{name: "v0 failed rejected", input: "failed", want: "", wantErr: true},
		{name: "v0 canceled rejected", input: "canceled", want: "", wantErr: true},

		// Other invalid inputs
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
// v1 canonical store.JobStatus values.
// v1 status model: Created, Queued, Running, Success, Fail, Cancelled.
func TestValidateJobStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    JobStatus
		wantErr bool
	}{
		// v1 canonical values (capitalized)
		{name: "Created", input: "Created", want: JobStatusCreated, wantErr: false},
		{name: "Queued", input: "Queued", want: JobStatusQueued, wantErr: false},
		{name: "Running", input: "Running", want: JobStatusRunning, wantErr: false},
		{name: "Success", input: "Success", want: JobStatusSuccess, wantErr: false},
		{name: "Fail", input: "Fail", want: JobStatusFail, wantErr: false},
		{name: "Cancelled", input: "Cancelled", want: JobStatusCancelled, wantErr: false},

		// v0 lowercase values should fail validation in v1
		{name: "v0 created invalid", input: "created", want: "", wantErr: true},
		{name: "v0 pending invalid", input: "pending", want: "", wantErr: true},
		{name: "v0 succeeded invalid", input: "succeeded", want: "", wantErr: true},
		{name: "v0 failed invalid", input: "failed", want: "", wantErr: true},
		{name: "v0 skipped invalid", input: "skipped", want: "", wantErr: true},
		{name: "v0 canceled invalid", input: "canceled", want: "", wantErr: true},
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
// v1 canonical store.RunStatus values.
// v1 status model: Started, Cancelled, Finished.
func TestValidateRunStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    RunStatus
		wantErr bool
	}{
		// v1 canonical values (capitalized)
		{name: "Started", input: "Started", want: RunStatusStarted, wantErr: false},
		{name: "Cancelled", input: "Cancelled", want: RunStatusCancelled, wantErr: false},
		{name: "Finished", input: "Finished", want: RunStatusFinished, wantErr: false},

		// v0 lowercase values should fail validation in v1
		{name: "v0 queued invalid", input: "queued", want: "", wantErr: true},
		{name: "v0 assigned invalid", input: "assigned", want: "", wantErr: true},
		{name: "v0 running invalid", input: "running", want: "", wantErr: true},
		{name: "v0 succeeded invalid", input: "succeeded", want: "", wantErr: true},
		{name: "v0 failed invalid", input: "failed", want: "", wantErr: true},
		{name: "v0 canceled invalid", input: "canceled", want: "", wantErr: true},
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

// TestValidateRunRepoStatus verifies that ValidateRunRepoStatus correctly validates
// v1 canonical store.RunRepoStatus values for per-repo execution state in batched runs.
// v1 status model: Queued, Running, Cancelled, Fail, Success.
func TestValidateRunRepoStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    RunRepoStatus
		wantErr bool
	}{
		// v1 canonical values (capitalized)
		{name: "Queued", input: "Queued", want: RunRepoStatusQueued, wantErr: false},
		{name: "Running", input: "Running", want: RunRepoStatusRunning, wantErr: false},
		{name: "Cancelled", input: "Cancelled", want: RunRepoStatusCancelled, wantErr: false},
		{name: "Fail", input: "Fail", want: RunRepoStatusFail, wantErr: false},
		{name: "Success", input: "Success", want: RunRepoStatusSuccess, wantErr: false},

		// v0 lowercase values should fail validation in v1
		{name: "v0 pending invalid", input: "pending", want: "", wantErr: true},
		{name: "v0 running invalid", input: "running", want: "", wantErr: true},
		{name: "v0 succeeded invalid", input: "succeeded", want: "", wantErr: true},
		{name: "v0 failed invalid", input: "failed", want: "", wantErr: true},
		{name: "v0 skipped invalid", input: "skipped", want: "", wantErr: true},
		{name: "v0 cancelled invalid", input: "cancelled", want: "", wantErr: true},
		{name: "unknown", input: "unknown", want: "", wantErr: true},
		{name: "empty", input: "", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidateRunRepoStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRunRepoStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateRunRepoStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestConvertToRunRepoStatus verifies that ConvertToRunRepoStatus correctly maps
// v1 canonical strings to store.RunRepoStatus values.
// v1 status model: Queued, Running, Cancelled, Fail, Success.
func TestConvertToRunRepoStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    RunRepoStatus
		wantErr bool
	}{
		// v1 canonical values (capitalized)
		{name: "Queued", input: "Queued", want: RunRepoStatusQueued, wantErr: false},
		{name: "Running", input: "Running", want: RunRepoStatusRunning, wantErr: false},
		{name: "Cancelled", input: "Cancelled", want: RunRepoStatusCancelled, wantErr: false},
		{name: "Fail", input: "Fail", want: RunRepoStatusFail, wantErr: false},
		{name: "Success", input: "Success", want: RunRepoStatusSuccess, wantErr: false},

		// v0 lowercase values should be rejected in v1
		{name: "v0 pending rejected", input: "pending", want: "", wantErr: true},
		{name: "v0 running rejected", input: "running", want: "", wantErr: true},
		{name: "v0 succeeded rejected", input: "succeeded", want: "", wantErr: true},
		{name: "v0 failed rejected", input: "failed", want: "", wantErr: true},
		{name: "v0 skipped rejected", input: "skipped", want: "", wantErr: true},
		{name: "v0 cancelled rejected", input: "cancelled", want: "", wantErr: true},

		// Other invalid inputs
		{name: "unknown", input: "unknown", want: "", wantErr: true},
		{name: "empty", input: "", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertToRunRepoStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertToRunRepoStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ConvertToRunRepoStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
