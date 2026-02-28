package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestExecuteRun_InferJobTypeFromJobName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		jobName string
		want    types.JobType
	}{
		{name: "pre gate", jobName: "pre-gate", want: types.JobTypePreGate},
		{name: "post gate", jobName: "post-gate", want: types.JobTypePostGate},
		{name: "re gate", jobName: "re-gate-1", want: types.JobTypeReGate},
		{name: "heal", jobName: "heal-1", want: types.JobTypeHeal},
		{name: "mod", jobName: "mig-0", want: types.JobTypeMod},
		{name: "mr", jobName: "mr", want: types.JobTypeMR},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := inferJobTypeFromJobName(tc.jobName); got != tc.want {
				t.Fatalf("inferJobTypeFromJobName(%q) = %q, want %q", tc.jobName, got, tc.want)
			}
		})
	}
}

func TestExecuteRun_InferJobTypeFromJobName_Unknown(t *testing.T) {
	t.Parallel()

	if got := inferJobTypeFromJobName("something-else"); got != "" {
		t.Fatalf("inferJobTypeFromJobName(unknown) = %q, want empty", got)
	}
}
