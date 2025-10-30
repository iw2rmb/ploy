package contracts

import (
	"context"
	"fmt"
	"strings"
)

type InMemoryBus struct {
    ClaimedTickets []string
    Checkpoints    []WorkflowCheckpoint
    Artifacts      []WorkflowArtifact
    tickets        []string
    Manifest       ManifestReference
    Repo           RepoMaterialization
}

func NewInMemoryBus() *InMemoryBus {
    return &InMemoryBus{}
}

func (b *InMemoryBus) EnqueueTicket(ticketID string) {
    b.tickets = append(b.tickets, ticketID)
}

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
    return WorkflowTicket{SchemaVersion: SchemaVersion, TicketID: trimmed, Manifest: manifest, Repo: b.Repo}, nil
}

func (b *InMemoryBus) PublishCheckpoint(ctx context.Context, checkpoint WorkflowCheckpoint) error {
	_ = ctx
	b.Checkpoints = append(b.Checkpoints, checkpoint)
	return nil
}

func (b *InMemoryBus) PublishArtifact(ctx context.Context, artifact WorkflowArtifact) error {
	_ = ctx
	if err := artifact.Validate(); err != nil {
		return err
	}
	b.Artifacts = append(b.Artifacts, artifact)
	return nil
}

// RecordedCheckpoints returns a copy of the checkpoints published to the in-memory bus.
func (b *InMemoryBus) RecordedCheckpoints() []WorkflowCheckpoint {
	if b == nil {
		return nil
	}
	dup := make([]WorkflowCheckpoint, len(b.Checkpoints))
	copy(dup, b.Checkpoints)
	return dup
}
