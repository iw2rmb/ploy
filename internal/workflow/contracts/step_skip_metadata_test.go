package contracts

import (
	"testing"
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
				RefRepoSHAOut: "0123456789abcdef0123456789abcdef01234567",
				Hash:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
		{
			name: "invalid repo sha out",
			meta: &MigStepSkipMetadata{
				Enabled:       true,
				RefRepoSHAOut: "bad-sha",
			},
			wantErr: true,
		},
		{
			name: "invalid hash",
			meta: &MigStepSkipMetadata{
				Enabled:       true,
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
