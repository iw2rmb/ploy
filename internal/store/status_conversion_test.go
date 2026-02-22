package store

import (
	"testing"
)

func TestConvertToJobStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    JobStatus
		wantErr bool
	}{
		{name: "Created", input: "Created", want: JobStatusCreated},
		{name: "Queued", input: "Queued", want: JobStatusQueued},
		{name: "Running", input: "Running", want: JobStatusRunning},
		{name: "Success", input: "Success", want: JobStatusSuccess},
		{name: "Fail", input: "Fail", want: JobStatusFail},
		{name: "Cancelled", input: "Cancelled", want: JobStatusCancelled},

		// v0 lowercase values should be rejected in v1
		{name: "v0 created rejected", input: "created", want: "", wantErr: true},
		{name: "v0 pending rejected", input: "pending", want: "", wantErr: true},
		{name: "v0 running rejected", input: "running", want: "", wantErr: true},
		{name: "v0 succeeded rejected", input: "succeeded", want: "", wantErr: true},
		{name: "v0 failed rejected", input: "failed", want: "", wantErr: true},
		{name: "v0 skipped rejected", input: "skipped", want: "", wantErr: true},
		{name: "v0 canceled rejected", input: "canceled", want: "", wantErr: true},

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
