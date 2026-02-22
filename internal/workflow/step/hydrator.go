package step

import (
	"context"
	"errors"
	"fmt"

	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type filesystemWorkspaceHydrator struct {
	fetcher hydration.GitFetcher
}

// NewFilesystemWorkspaceHydrator creates a new workspace hydrator.
func NewFilesystemWorkspaceHydrator(opts FilesystemWorkspaceHydratorOptions) (WorkspaceHydrator, error) {
	if opts.RepoFetcher == nil {
		return nil, errors.New("repo fetcher is required")
	}
	return &filesystemWorkspaceHydrator{fetcher: opts.RepoFetcher}, nil
}

// Hydrate prepares the workspace by fetching repository sources as needed.
func (h *filesystemWorkspaceHydrator) Hydrate(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
	// Process each input that has repository hydration configured.
	for _, input := range manifest.Inputs {
		if input.Hydration != nil && input.Hydration.Repo != nil {
			if err := h.fetcher.Fetch(ctx, input.Hydration.Repo, workspace); err != nil {
				return fmt.Errorf("failed to hydrate input %s: %w", input.Name, err)
			}
		}
	}
	return nil
}
