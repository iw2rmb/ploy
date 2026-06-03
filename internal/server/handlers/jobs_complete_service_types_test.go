package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRouteCompletionServiceType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		jobType domaintypes.JobType
		want    completionServiceType
		wantOK  bool
	}{
		{name: "pre_gate", jobType: domaintypes.JobTypePreGate, want: completionServiceTypeGate, wantOK: true},
		{name: "post_gate", jobType: domaintypes.JobTypePostGate, want: completionServiceTypeGate, wantOK: true},
		{name: "mig", jobType: domaintypes.JobTypeMig, want: completionServiceTypeStep, wantOK: true},
		{name: "unknown", jobType: domaintypes.JobType("unknown"), want: "", wantOK: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := routeCompletionServiceType(tc.jobType)
			if ok != tc.wantOK {
				t.Fatalf("routeCompletionServiceType(%q) ok = %v, want %v", tc.jobType, ok, tc.wantOK)
			}
			if got != tc.want {
				t.Fatalf("routeCompletionServiceType(%q) type = %q, want %q", tc.jobType, got, tc.want)
			}
		})
	}
}
