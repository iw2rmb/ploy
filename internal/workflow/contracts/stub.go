package contracts

import (
	"context"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// InMemoryBus is a non-persistent test/local stub of the workflow
// events bus. It records claimed runs, checkpoints, and artifacts
// in memory only. It is not concurrency-safe and must not be used in
// production paths.
type InMemoryBus struct {
	ClaimedRuns []string
	Checkpoints []WorkflowCheckpoint
	Artifacts   []WorkflowArtifact
	runs        []string
	Manifest    ManifestReference
	Repo        RepoMaterialization
}

// NewInMemoryBus constructs an empty InMemoryBus.
func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{}
}

// EnqueueRun queues a run ID to be returned by ClaimRun
// when callers pass a blank ID.
func (b *InMemoryBus) EnqueueRun(runID string) {
	b.runs = append(b.runs, runID)
}

// ClaimRun returns a WorkflowRun for the provided ID. When the
// ID is blank, it pops from the internal queue or generates
// "run-auto-N". A default manifest is applied when none is set.
func (b *InMemoryBus) ClaimRun(ctx context.Context, runID string) (WorkflowRun, error) {
	_ = ctx
	trimmed := strings.TrimSpace(runID)
	if trimmed == "" {
		if len(b.runs) > 0 {
			trimmed = b.runs[0]
			b.runs = b.runs[1:]
		} else {
			trimmed = fmt.Sprintf("run-auto-%d", len(b.ClaimedRuns)+1)
		}
	}
	b.ClaimedRuns = append(b.ClaimedRuns, trimmed)
	manifest := b.Manifest
	if manifest.Name == "" || manifest.Version == "" {
		manifest = ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
	return WorkflowRun{SchemaVersion: SchemaVersion, RunID: types.RunID(trimmed), Manifest: manifest, Repo: b.Repo}, nil
}

// PublishCheckpoint records a checkpoint in memory only.
func (b *InMemoryBus) PublishCheckpoint(ctx context.Context, checkpoint WorkflowCheckpoint) error {
	_ = ctx
	b.Checkpoints = append(b.Checkpoints, checkpoint)
	return nil
}

// PublishArtifact validates and records an artifact envelope in memory.
func (b *InMemoryBus) PublishArtifact(ctx context.Context, artifact WorkflowArtifact) error {
	_ = ctx
	if err := artifact.Validate(); err != nil {
		return err
	}
	b.Artifacts = append(b.Artifacts, artifact)
	return nil
}

// RecordedCheckpoints returns a copy of checkpoints published to the
// in-memory bus; useful for assertions in tests.
func (b *InMemoryBus) RecordedCheckpoints() []WorkflowCheckpoint {
	if b == nil {
		return nil
	}
	dup := make([]WorkflowCheckpoint, len(b.Checkpoints))
	copy(dup, b.Checkpoints)
	return dup
}
