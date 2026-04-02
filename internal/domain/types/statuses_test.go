package types

import "testing"

func TestParseJobStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    JobStatus
		wantErr bool
	}{
		{name: "created", input: "Created", want: JobStatusCreated},
		{name: "queued", input: "Queued", want: JobStatusQueued},
		{name: "running", input: "Running", want: JobStatusRunning},
		{name: "success", input: "Success", want: JobStatusSuccess},
		{name: "fail", input: "Fail", want: JobStatusFail},
		{name: "error", input: "Error", want: JobStatusError},
		{name: "cancelled", input: "Cancelled", want: JobStatusCancelled},
		{name: "invalid lowercase", input: "running", wantErr: true},
		{name: "unknown", input: "unknown", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseJobStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseJobStatus(%q) error=%v wantErr=%v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("ParseJobStatus(%q)=%q want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStatusScanRejectsUnknown(t *testing.T) {
	t.Parallel()

	t.Run("job", func(t *testing.T) {
		t.Parallel()
		var status JobStatus
		if err := status.Scan("Running"); err != nil {
			t.Fatalf("Scan valid job status failed: %v", err)
		}
		if status != JobStatusRunning {
			t.Fatalf("job status=%q want %q", status, JobStatusRunning)
		}
		if err := status.Scan("running"); err == nil {
			t.Fatal("expected lowercase job status scan to fail")
		}
	})

	t.Run("run", func(t *testing.T) {
		t.Parallel()
		var status RunStatus
		if err := status.Scan([]byte("Finished")); err != nil {
			t.Fatalf("Scan valid run status failed: %v", err)
		}
		if status != RunStatusFinished {
			t.Fatalf("run status=%q want %q", status, RunStatusFinished)
		}
		if err := status.Scan("finished"); err == nil {
			t.Fatal("expected lowercase run status scan to fail")
		}
	})

	t.Run("run_repo", func(t *testing.T) {
		t.Parallel()
		var status RunRepoStatus
		if err := status.Scan("Success"); err != nil {
			t.Fatalf("Scan valid run repo status failed: %v", err)
		}
		if status != RunRepoStatusSuccess {
			t.Fatalf("run repo status=%q want %q", status, RunRepoStatusSuccess)
		}
		if err := status.Scan("success"); err == nil {
			t.Fatal("expected lowercase run repo status scan to fail")
		}
	})
}

func TestDiffJobTypeValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   DiffJobType
		wantErr bool
	}{
		{name: "mig", input: DiffJobTypeMig},
		{name: "healing", input: DiffJobTypeHealing},
		{name: "invalid", input: DiffJobType("other"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate(%q) error=%v wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}
