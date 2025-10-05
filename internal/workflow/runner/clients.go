package runner

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

// GridClient executes individual workflow stages on Grid.
type GridClient interface {
	ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage Stage, workspace string) (StageOutcome, error)
	CancelWorkflow(ctx context.Context, req CancelRequest) (CancelResult, error)
}

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
