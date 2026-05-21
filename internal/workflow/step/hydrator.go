package step

import (
	"context"
	"errors"
	"fmt"

	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type filesystemWorkspaceHydrator struct {
	fetcher hydration.GitFetcher
}

// NewFilesystemWorkspaceHydrator creates a new workspace hydrator backed by the given fetcher.
func NewFilesystemWorkspaceHydrator(fetcher hydration.GitFetcher) (WorkspaceHydrator, error) {
	if fetcher == nil {
		return nil, errors.New("repo fetcher is required")
	}
	return &filesystemWorkspaceHydrator{fetcher: fetcher}, nil
}

// Hydrate prepares the workspace by fetching repository sources as needed.
func (h *filesystemWorkspaceHydrator) Hydrate(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
	auth := gitAuthOptionsFromManifest(manifest)
	// Process each input that has repository hydration configured.
	for _, input := range manifest.Inputs {
		if input.Hydration != nil && input.Hydration.Repo != nil {
			if err := h.fetcher.Fetch(ctx, input.Hydration.Repo, workspace, auth); err != nil {
				return fmt.Errorf("failed to hydrate input %s: %w", input.Name, err)
			}
		}
	}
	return nil
}

func gitAuthOptionsFromManifest(manifest contracts.StepManifest) gitauth.Options {
	opts := gitauth.Options{}
	if pat, ok := manifest.OptionString("gitlab_pat"); ok {
		opts.GitLabPAT = pat
	}
	if domain, ok := manifest.OptionString("gitlab_domain"); ok {
		opts.GitLabDomain = domain
	}
	return opts
}
