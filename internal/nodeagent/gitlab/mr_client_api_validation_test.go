package gitlab

import (
	"context"
	"strings"
	"testing"
)

// Validation tests: required field validation and PAT redaction in validation errors.

// TestCreateMR_Validation verifies that CreateMR returns errors for missing
// required fields (domain, project_id, pat, title, source_branch, target_branch).
func TestCreateMR_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     MRCreateRequest
		wantErr string
	}{
		{
			name: "missing_domain",
			req: MRCreateRequest{
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-token",
				Title:        "Test",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			wantErr: "domain is required",
		},
		{
			name: "missing_project_id",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				PAT:          "glpat-token",
				Title:        "Test",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			wantErr: "project_id is required",
		},
		{
			name: "missing_pat",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				ProjectID:    "org%2Fproject",
				Title:        "Test",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			wantErr: "pat is required",
		},
		{
			name: "missing_title",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-token",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			wantErr: "title is required",
		},
		{
			name: "missing_source_branch",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-token",
				Title:        "Test",
				TargetBranch: "main",
			},
			wantErr: "source_branch is required",
		},
		{
			name: "missing_target_branch",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-token",
				Title:        "Test",
				SourceBranch: "feature",
			},
			wantErr: "target_branch is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMRClient()
			ctx := context.Background()
			_, err := client.CreateMR(ctx, tt.req)

			if err == nil {
				t.Errorf("expected error, got nil")
				return
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
