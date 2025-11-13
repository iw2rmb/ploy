package gitlab

import (
	"testing"
)

// TestExtractProjectIDFromURL verifies URL parsing and project ID extraction
// from GitLab URLs, including nested paths and various URL formats.
func TestExtractProjectIDFromURL(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		wantID  string
		wantErr bool
	}{
		{
			name:    "standard_https_url",
			repoURL: "https://gitlab.com/org/project.git",
			wantID:  "org%2Fproject",
			wantErr: false,
		},
		{
			name:    "without_git_suffix",
			repoURL: "https://gitlab.com/org/project",
			wantID:  "org%2Fproject",
			wantErr: false,
		},
		{
			name:    "nested_path",
			repoURL: "https://gitlab.example.com/group/subgroup/project.git",
			wantID:  "group%2Fsubgroup%2Fproject",
			wantErr: false,
		},
		{
			name:    "self_hosted",
			repoURL: "https://gitlab.internal.net/team/repo.git",
			wantID:  "team%2Frepo",
			wantErr: false,
		},
		{
			name:    "empty_path",
			repoURL: "https://gitlab.com/",
			wantID:  "",
			wantErr: true,
		},
		{
			name:    "invalid_url",
			repoURL: "not a valid url",
			wantID:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := ExtractProjectIDFromURL(tt.repoURL)

			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractProjectIDFromURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotID != tt.wantID {
				t.Errorf("ExtractProjectIDFromURL() = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}
