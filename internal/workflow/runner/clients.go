package runner

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

// RuntimeClient executes individual workflow stages using the active runtime.
type RuntimeClient interface {
	ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage Stage, workspace string) (StageOutcome, error)
	CancelWorkflow(ctx context.Context, req CancelRequest) (CancelResult, error)
}

// GridClient retained as an alias for backward compatibility during migration.
// Prefer RuntimeClient in new code.
type GridClient = RuntimeClient

// EventsClient brokers ticket claims, checkpoints, and artifact publication.
type EventsClient interface {
	ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error)
	PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error
	PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error
}

// ManifestCompiler expands manifest references into executable manifests.
type ManifestCompiler interface {
	Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error)
}
