package contracts

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestMigStepSkipMetadataValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		meta := &MigStepSkipMetadata{
			Enabled:       true,
			RefJobID:      types.JobID("2w6vNfL9qYHhM7xQ8TzP1bK3n4D"),
			RefRepoSHAOut: "0123456789abcdef0123456789abcdef01234567",
			Hash:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		if err := meta.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("missing ref job id", func(t *testing.T) {
		meta := &MigStepSkipMetadata{
			Enabled:       true,
			RefRepoSHAOut: "0123456789abcdef0123456789abcdef01234567",
		}
		if err := meta.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want non-nil")
		}
	})

	t.Run("invalid repo sha out", func(t *testing.T) {
		meta := &MigStepSkipMetadata{
			Enabled:       true,
			RefJobID:      types.JobID("2w6vNfL9qYHhM7xQ8TzP1bK3n4D"),
			RefRepoSHAOut: "bad-sha",
		}
		if err := meta.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want non-nil")
		}
	})

	t.Run("invalid hash", func(t *testing.T) {
		meta := &MigStepSkipMetadata{
			Enabled:       true,
			RefJobID:      types.JobID("2w6vNfL9qYHhM7xQ8TzP1bK3n4D"),
			RefRepoSHAOut: "0123456789abcdef0123456789abcdef01234567",
			Hash:          "not-a-hash",
		}
		if err := meta.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want non-nil")
		}
	})
}
