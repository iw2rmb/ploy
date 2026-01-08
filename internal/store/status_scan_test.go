package store

import (
	"testing"
)

// TestJobStatusScanRejectsUnknown verifies that JobStatus.Scan returns an error
// on unknown string values, failing fast on schema/code drift.
func TestJobStatusScanRejectsUnknown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
		want    JobStatus
	}{
		// Valid values should scan successfully
		{name: "Created", input: "Created", wantErr: false, want: JobStatusCreated},
		{name: "Queued", input: "Queued", wantErr: false, want: JobStatusQueued},
		{name: "Running", input: "Running", wantErr: false, want: JobStatusRunning},
		{name: "Success", input: "Success", wantErr: false, want: JobStatusSuccess},
		{name: "Fail", input: "Fail", wantErr: false, want: JobStatusFail},
		{name: "Cancelled", input: "Cancelled", wantErr: false, want: JobStatusCancelled},
		// Bytes should also work
		{name: "Created bytes", input: []byte("Created"), wantErr: false, want: JobStatusCreated},
		{name: "Success bytes", input: []byte("Success"), wantErr: false, want: JobStatusSuccess},
		// Unknown values should be rejected
		{name: "unknown string", input: "Unknown", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "lowercase created", input: "created", wantErr: true},
		{name: "v0 pending", input: "pending", wantErr: true},
		{name: "v0 succeeded", input: "succeeded", wantErr: true},
		{name: "v0 failed", input: "failed", wantErr: true},
		{name: "v0 skipped", input: "skipped", wantErr: true},
		{name: "arbitrary string", input: "NotAStatus", wantErr: true},
		{name: "unknown bytes", input: []byte("Unknown"), wantErr: true},
		// Wrong type should be rejected
		{name: "int type", input: 42, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var status JobStatus
			err := status.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("JobStatus.Scan(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && status != tt.want {
				t.Errorf("JobStatus.Scan(%v) = %v, want %v", tt.input, status, tt.want)
			}
		})
	}
}

// TestRunStatusScanRejectsUnknown verifies that RunStatus.Scan returns an error
// on unknown string values, failing fast on schema/code drift.
func TestRunStatusScanRejectsUnknown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
		want    RunStatus
	}{
		// Valid values should scan successfully
		{name: "Started", input: "Started", wantErr: false, want: RunStatusStarted},
		{name: "Cancelled", input: "Cancelled", wantErr: false, want: RunStatusCancelled},
		{name: "Finished", input: "Finished", wantErr: false, want: RunStatusFinished},
		// Bytes should also work
		{name: "Started bytes", input: []byte("Started"), wantErr: false, want: RunStatusStarted},
		{name: "Finished bytes", input: []byte("Finished"), wantErr: false, want: RunStatusFinished},
		// Unknown values should be rejected
		{name: "unknown string", input: "Unknown", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "lowercase started", input: "started", wantErr: true},
		{name: "v0 queued", input: "queued", wantErr: true},
		{name: "v0 assigned", input: "assigned", wantErr: true},
		{name: "v0 running", input: "running", wantErr: true},
		{name: "v0 succeeded", input: "succeeded", wantErr: true},
		{name: "v0 failed", input: "failed", wantErr: true},
		{name: "arbitrary string", input: "NotAStatus", wantErr: true},
		{name: "unknown bytes", input: []byte("Unknown"), wantErr: true},
		// Wrong type should be rejected
		{name: "int type", input: 42, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var status RunStatus
			err := status.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunStatus.Scan(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && status != tt.want {
				t.Errorf("RunStatus.Scan(%v) = %v, want %v", tt.input, status, tt.want)
			}
		})
	}
}

// TestRunRepoStatusScanRejectsUnknown verifies that RunRepoStatus.Scan returns an error
// on unknown string values, failing fast on schema/code drift.
func TestRunRepoStatusScanRejectsUnknown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
		want    RunRepoStatus
	}{
		// Valid values should scan successfully
		{name: "Queued", input: "Queued", wantErr: false, want: RunRepoStatusQueued},
		{name: "Running", input: "Running", wantErr: false, want: RunRepoStatusRunning},
		{name: "Cancelled", input: "Cancelled", wantErr: false, want: RunRepoStatusCancelled},
		{name: "Fail", input: "Fail", wantErr: false, want: RunRepoStatusFail},
		{name: "Success", input: "Success", wantErr: false, want: RunRepoStatusSuccess},
		// Bytes should also work
		{name: "Queued bytes", input: []byte("Queued"), wantErr: false, want: RunRepoStatusQueued},
		{name: "Success bytes", input: []byte("Success"), wantErr: false, want: RunRepoStatusSuccess},
		// Unknown values should be rejected
		{name: "unknown string", input: "Unknown", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "lowercase queued", input: "queued", wantErr: true},
		{name: "v0 pending", input: "pending", wantErr: true},
		{name: "v0 succeeded", input: "succeeded", wantErr: true},
		{name: "v0 failed", input: "failed", wantErr: true},
		{name: "v0 skipped", input: "skipped", wantErr: true},
		{name: "arbitrary string", input: "NotAStatus", wantErr: true},
		{name: "unknown bytes", input: []byte("Unknown"), wantErr: true},
		// Wrong type should be rejected
		{name: "int type", input: 42, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var status RunRepoStatus
			err := status.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunRepoStatus.Scan(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && status != tt.want {
				t.Errorf("RunRepoStatus.Scan(%v) = %v, want %v", tt.input, status, tt.want)
			}
		})
	}
}
