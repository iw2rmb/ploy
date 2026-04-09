package contracts

import (
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestSBOMStepSkipMetadataValidate(t *testing.T) {
	t.Parallel()

	valid := &SBOMStepSkipMetadata{
		Enabled:       true,
		RefJobID:      types.JobID("2Y3Y6cnx74Y4LnR2t9L6nWqv0x0"),
		RefArtifactID: "123e4567-e89b-12d3-a456-426614174000",
		RefJobImage:   "ghcr.io/iw2rmb/ploy/sbom-maven:latest",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}

	tests := []struct {
		name string
		meta *SBOMStepSkipMetadata
		want string
	}{
		{
			name: "disabled",
			meta: &SBOMStepSkipMetadata{
				Enabled:       false,
				RefJobID:      valid.RefJobID,
				RefArtifactID: valid.RefArtifactID,
				RefJobImage:   valid.RefJobImage,
			},
			want: "enabled",
		},
		{
			name: "missing ref job id",
			meta: &SBOMStepSkipMetadata{
				Enabled:       true,
				RefArtifactID: valid.RefArtifactID,
				RefJobImage:   valid.RefJobImage,
			},
			want: "ref_job_id",
		},
		{
			name: "bad artifact id",
			meta: &SBOMStepSkipMetadata{
				Enabled:       true,
				RefJobID:      valid.RefJobID,
				RefArtifactID: "not-uuid",
				RefJobImage:   valid.RefJobImage,
			},
			want: "ref_artifact_id",
		},
		{
			name: "missing image",
			meta: &SBOMStepSkipMetadata{
				Enabled:       true,
				RefJobID:      valid.RefJobID,
				RefArtifactID: valid.RefArtifactID,
			},
			want: "ref_job_image",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.meta.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() err = %v, want substring %q", err, tc.want)
			}
		})
	}
}
