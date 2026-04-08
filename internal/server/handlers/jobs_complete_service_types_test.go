package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRouteCompleteJobServiceType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		jobType domaintypes.JobType
		want    completeJobServiceType
		wantOK  bool
	}{
		{name: "pre_gate", jobType: domaintypes.JobTypePreGate, want: completeJobServiceTypeGate, wantOK: true},
		{name: "post_gate", jobType: domaintypes.JobTypePostGate, want: completeJobServiceTypeGate, wantOK: true},
		{name: "re_gate", jobType: domaintypes.JobTypeReGate, want: completeJobServiceTypeGate, wantOK: true},
		{name: "mig", jobType: domaintypes.JobTypeMig, want: completeJobServiceTypeStep, wantOK: true},
		{name: "heal", jobType: domaintypes.JobTypeHeal, want: completeJobServiceTypeStep, wantOK: true},
		{name: "sbom", jobType: domaintypes.JobTypeSBOM, want: completeJobServiceTypeSBOM, wantOK: true},
		{name: "hook", jobType: domaintypes.JobTypeHook, want: completeJobServiceTypeHook, wantOK: true},
		{name: "mr", jobType: domaintypes.JobTypeMR, want: completeJobServiceTypeMR, wantOK: true},
		{name: "unknown", jobType: domaintypes.JobType("unknown"), want: "", wantOK: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := routeCompleteJobServiceType(tc.jobType)
			if ok != tc.wantOK {
				t.Fatalf("routeCompleteJobServiceType(%q) ok = %v, want %v", tc.jobType, ok, tc.wantOK)
			}
			if got != tc.want {
				t.Fatalf("routeCompleteJobServiceType(%q) type = %q, want %q", tc.jobType, got, tc.want)
			}
		})
	}
}
