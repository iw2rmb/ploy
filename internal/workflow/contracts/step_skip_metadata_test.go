package contracts

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestMigStepSkipMetadataValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		meta    *MigStepSkipMetadata
		wantErr bool
	}{
		{
			name: "valid",
			meta: &MigStepSkipMetadata{
				Enabled:       true,
				RefJobID:      types.JobID("2w6vNfL9qYHhM7xQ8TzP1bK3n4D"),
				RefRepoSHAOut: "0123456789abcdef0123456789abcdef01234567",
				Hash:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
		{
			name: "missing ref job id",
			meta: &MigStepSkipMetadata{
				Enabled:       true,
				RefRepoSHAOut: "0123456789abcdef0123456789abcdef01234567",
			},
			wantErr: true,
		},
		{
			name: "invalid repo sha out",
			meta: &MigStepSkipMetadata{
				Enabled:       true,
				RefJobID:      types.JobID("2w6vNfL9qYHhM7xQ8TzP1bK3n4D"),
				RefRepoSHAOut: "bad-sha",
			},
			wantErr: true,
		},
		{
			name: "invalid hash",
			meta: &MigStepSkipMetadata{
				Enabled:       true,
				RefJobID:      types.JobID("2w6vNfL9qYHhM7xQ8TzP1bK3n4D"),
				RefRepoSHAOut: "0123456789abcdef0123456789abcdef01234567",
				Hash:          "not-a-hash",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.meta.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
