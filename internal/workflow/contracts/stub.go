package contracts

import (
	"context"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// InMemoryBus is a non-persistent test/local stub of the workflow
// events bus. It records claimed tickets, checkpoints, and artifacts
// in memory only. It is not concurrency-safe and must not be used in
// production paths.
type InMemoryBus struct {
	ClaimedTickets []string
	Checkpoints    []WorkflowCheckpoint
	Artifacts      []WorkflowArtifact
	tickets        []string
	Manifest       ManifestReference
	Repo           RepoMaterialization
}

// NewInMemoryBus constructs an empty InMemoryBus.
func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{}
}

// EnqueueTicket queues a ticket ID to be returned by ClaimTicket
// when callers pass a blank ID.
func (b *InMemoryBus) EnqueueTicket(ticketID string) {
	b.tickets = append(b.tickets, ticketID)
}

// ClaimTicket returns a WorkflowTicket for the provided ID. When the
// ID is blank, it pops from the internal queue or generates
// "ticket-auto-N". A default manifest is applied when none is set.
func (b *InMemoryBus) ClaimTicket(ctx context.Context, ticketID string) (WorkflowTicket, error) {
	_ = ctx
	trimmed := strings.TrimSpace(ticketID)
	if trimmed == "" {
		if len(b.tickets) > 0 {
			trimmed = b.tickets[0]
			b.tickets = b.tickets[1:]
		} else {
			trimmed = fmt.Sprintf("ticket-auto-%d", len(b.ClaimedTickets)+1)
		}
	}
	b.ClaimedTickets = append(b.ClaimedTickets, trimmed)
	manifest := b.Manifest
	if manifest.Name == "" || manifest.Version == "" {
		manifest = ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
	return WorkflowTicket{SchemaVersion: SchemaVersion, TicketID: types.TicketID(trimmed), Manifest: manifest, Repo: b.Repo}, nil
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
