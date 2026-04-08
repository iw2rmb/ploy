package types

import "testing"

func TestJobTypeValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		jobType JobType
		wantErr bool
	}{
		{name: "pre_gate", jobType: JobTypePreGate},
		{name: "mig", jobType: JobTypeMig},
		{name: "post_gate", jobType: JobTypePostGate},
		{name: "heal", jobType: JobTypeHeal},
		{name: "re_gate", jobType: JobTypeReGate},
		{name: "sbom", jobType: JobTypeSBOM},
		{name: "hook", jobType: JobTypeHook},
		{name: "mr", jobType: JobTypeMR},
		{name: "invalid", jobType: JobType("legacy_gate"), wantErr: true},
		{name: "empty", jobType: JobType(""), wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.jobType.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("Validate() error = nil, want non-nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}
