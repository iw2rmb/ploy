package lifecycle_test

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func TestJobStatusFromExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exitCode int
		want     domaintypes.JobStatus
	}{
		{name: "zero is success", exitCode: 0, want: domaintypes.JobStatusSuccess},
		{name: "one is fail", exitCode: 1, want: domaintypes.JobStatusFail},
		{name: "exit above one is error", exitCode: 2, want: domaintypes.JobStatusError},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := lifecycle.JobStatusFromExitCode(tt.exitCode); got != tt.want {
				t.Fatalf("JobStatusFromExitCode(%d) = %q, want %q", tt.exitCode, got, tt.want)
			}
		})
	}
}
