package snapshot

import (
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/gitlabtoken"
	"github.com/iw2rmb/ploy/internal/server/gitlabtokens"
)

func TestServiceAuthForMetadata(t *testing.T) {
	token := "glpat-snapshot-secret"
	hash := gitlabtoken.Hash(token)
	runID := types.NewRunID()

	tests := []struct {
		name       string
		meta       Metadata
		registry   *gitlabtokens.Registry
		wantToken  string
		wantDomain string
		wantErr    error
	}{
		{
			name:       "no marker uses default auth",
			meta:       Metadata{RepoURL: "https://gitlab.example.com/acme/service.git"},
			registry:   gitlabtokens.NewRegistry(),
			wantToken:  "default-token",
			wantDomain: "default.gitlab.example.com",
		},
		{
			name: "marker registry hit uses ephemeral token scoped to configured GitLab domain",
			meta: Metadata{
				RepoURL:         "https://gitlab.example.com/acme/service.git",
				GitLabTokenHash: hash,
			},
			registry: func() *gitlabtokens.Registry {
				r := gitlabtokens.NewRegistry()
				r.Register(hash, token, []types.RunID{runID})
				return r
			}(),
			wantToken:  token,
			wantDomain: "default.gitlab.example.com",
		},
		{
			name: "marker registry miss fails explicitly",
			meta: Metadata{
				RepoURL:         "https://gitlab.example.com/acme/service.git",
				GitLabTokenHash: hash,
			},
			registry: gitlabtokens.NewRegistry(),
			wantErr:  ErrEphemeralGitLabTokenUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(Options{
				Auth: gitauth.Options{
					GitLabPAT:    "default-token",
					GitLabDomain: "default.gitlab.example.com",
				},
				TokenLookup: tt.registry,
			})
			auth, err := svc.authForMetadata(tt.meta, tt.meta.RepoURL)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("authForMetadata() error = %v", err)
			}
			if auth.GitLabPAT != tt.wantToken {
				t.Fatalf("GitLabPAT = %q, want %q", auth.GitLabPAT, tt.wantToken)
			}
			if auth.GitLabDomain != tt.wantDomain {
				t.Fatalf("GitLabDomain = %q, want %q", auth.GitLabDomain, tt.wantDomain)
			}
		})
	}
}
