package contracts

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRepoMaterializationValidate(t *testing.T) {
	tests := []struct {
		name    string
		repo    RepoMaterialization
		wantErr bool
	}{
		{name: "empty is valid"},
		{
			name: "base ref",
			repo: RepoMaterialization{URL: types.RepoURL("https://gitlab.com/iw2rmb/sample.git"), BaseRef: types.GitRef("main")},
		},
		{
			name: "commit",
			repo: RepoMaterialization{URL: types.RepoURL("https://gitlab.com/iw2rmb/sample.git"), Commit: types.CommitSHA("abcdef1234567890")},
		},
		{
			name:    "URL requires ref or commit",
			repo:    RepoMaterialization{URL: types.RepoURL("https://example.com/repo.git")},
			wantErr: true,
		},
		{
			name:    "invalid URL scheme",
			repo:    RepoMaterialization{URL: types.RepoURL("http://example.com/repo.git")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.repo.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
