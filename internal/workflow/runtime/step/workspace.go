package step

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// WorkspaceHydrator prepares the workspace for execution.
type WorkspaceHydrator interface {
	Prepare(ctx context.Context, req WorkspaceRequest) (Workspace, error)
}

// WorkspaceRequest asks hydrator to materialise inputs.
type WorkspaceRequest struct {
	Manifest contracts.StepManifest
}

// Workspace describes hydrated paths.
type Workspace struct {
	Inputs             map[string]string
	WorkingDir         string
	HydrationSnapshots map[string]PublishedArtifact
}
