package nodeagent

import (
	"testing"
)

func TestMigStepIndexFromJobName_MultiStep(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		jobName string
		steps   int
		want    int
		wantErr bool
	}{
		{name: "step0", jobName: "mig-0", steps: 3, want: 0},
		{name: "step2", jobName: "mig-2", steps: 3, want: 2},
		{name: "single step non-indexed", jobName: "mig", steps: 1, want: 0},
		{name: "invalid prefix", jobName: "pre-gate", steps: 2, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := migStepIndexFromJobName(tc.jobName, tc.steps)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for job_name=%q", tc.jobName)
				}
				return
			}
			if err != nil {
				t.Fatalf("migStepIndexFromJobName(%q,%d) returned error: %v", tc.jobName, tc.steps, err)
			}
			if got != tc.want {
				t.Fatalf("migStepIndexFromJobName(%q,%d)=%d want %d", tc.jobName, tc.steps, got, tc.want)
			}
		})
	}
}
