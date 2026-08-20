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

func TestBuildGateMetadataValidate(t *testing.T) {
	meta := BuildGateStageMetadata{
		LogDigest: types.Sha256Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		StaticChecks: []BuildGateStaticCheckReport{{
			Language: "go",
			Tool:     "go vet",
			Passed:   false,
			Failures: []BuildGateStaticCheckFailure{{
				RuleID:   "GOVET001",
				File:     "internal/pkg/main.go",
				Line:     12,
				Column:   4,
				Severity: "warning",
				Message:  "unused result",
			}},
		}},
		LogFindings: []BuildGateLogFinding{{
			Code:     "git.auth",
			Severity: "error",
			Message:  "Authenticate Git fetch credentials for remote repository access.",
			Evidence: "fatal: unable to access 'https://example.com/repo'",
		}},
	}
	if err := meta.Validate(); err != nil {
		t.Fatalf("expected valid build gate metadata, got %v", err)
	}
}

func TestBuildGateMetadataValidateRejectsEmptyFailureMessage(t *testing.T) {
	meta := BuildGateStageMetadata{
		StaticChecks: []BuildGateStaticCheckReport{{
			Language: "go",
			Tool:     "go vet",
			Failures: []BuildGateStaticCheckFailure{{
				RuleID:   "GOVET001",
				File:     "main.go",
				Severity: "error",
			}},
		}},
	}
	if err := meta.Validate(); err == nil {
		t.Fatal("expected validation error for missing failure message")
	}
}

func TestBuildGateMetadataValidateRejectsInvalidLogFinding(t *testing.T) {
	meta := BuildGateStageMetadata{
		LogFindings: []BuildGateLogFinding{{
			Code:    "git.auth",
			Message: "missing severity",
		}},
	}
	if err := meta.Validate(); err == nil {
		t.Fatal("expected validation error for log finding without severity")
	}
}

// TestBuildGateValidateRequestValidate tests the ref-only BuildGateValidateRequest validation.
