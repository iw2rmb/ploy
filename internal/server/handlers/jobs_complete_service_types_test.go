package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestKnownCompletionJobType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		jobType domaintypes.JobType
		want    bool
	}{
		{name: "pre_gate", jobType: domaintypes.JobTypePreGate, want: true},
		{name: "post_gate", jobType: domaintypes.JobTypePostGate, want: true},
		{name: "mig", jobType: domaintypes.JobTypeMig, want: true},
		{name: "unknown", jobType: domaintypes.JobType("unknown"), want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := knownCompletionJobType(tc.jobType); got != tc.want {
				t.Fatalf("knownCompletionJobType(%q) = %v, want %v", tc.jobType, got, tc.want)
			}
		})
	}
}
