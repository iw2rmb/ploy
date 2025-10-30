package runner_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

type errorEvents struct {
	ticket      contracts.WorkflowTicket
	claimErr    error
	publishErr  error
	artifactErr error
}

func (e *errorEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	if e.claimErr != nil {
		return contracts.WorkflowTicket{}, e.claimErr
	}
	if e.ticket.TicketID == "" {
		e.ticket.TicketID = ticketID
	}
	if e.ticket.SchemaVersion == "" {
		e.ticket.SchemaVersion = contracts.SchemaVersion
	}
    // tenant removed
	if e.ticket.Manifest.Name == "" || e.ticket.Manifest.Version == "" {
		e.ticket.Manifest = contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
	if strings.TrimSpace(e.ticket.TicketID) == "" {
		e.ticket.TicketID = "ticket-auto"
	}
	return e.ticket, nil
}

func (e *errorEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	if e.publishErr != nil {
		return e.publishErr
	}
	return nil
}

func (e *errorEvents) PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error {
	_ = ctx
	if e.artifactErr != nil {
		return e.artifactErr
	}
	return nil
}

type countingEvents struct {
	ticket       contracts.WorkflowTicket
	failAt       int
	publishCount int
	err          error
}

func (c *countingEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	if c.ticket.SchemaVersion == "" {
		c.ticket.SchemaVersion = contracts.SchemaVersion
	}
	if c.ticket.TicketID == "" {
		c.ticket.TicketID = ticketID
	}
    // tenant removed
	if c.ticket.Manifest.Name == "" || c.ticket.Manifest.Version == "" {
		c.ticket.Manifest = contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
	return c.ticket, nil
}

func (c *countingEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	c.publishCount++
	if c.failAt > 0 && c.publishCount == c.failAt {
		if c.err == nil {
			c.err = errors.New("publish checkpoint failure")
		}
		return c.err
	}
	return nil
}

func (c *countingEvents) PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error {
	_ = ctx
	return nil
}

type stageStatusEntry struct {
	stage  string
	status runner.StageStatus
}

func extractStageStatuses(checkpoints []contracts.WorkflowCheckpoint) []stageStatusEntry {
	result := make([]stageStatusEntry, 0, len(checkpoints))
	for _, cp := range checkpoints {
		result = append(result, stageStatusEntry{stage: cp.Stage, status: runner.StageStatus(cp.Status)})
	}
	return result
}

func collectStageStatuses(sequence []stageStatusEntry, stage string) []runner.StageStatus {
	statuses := make([]runner.StageStatus, 0, len(sequence))
	for _, entry := range sequence {
		if entry.stage == stage {
			statuses = append(statuses, entry.status)
		}
	}
	return statuses
}

func requireStageStatuses(t *testing.T, sequence []stageStatusEntry, stage string, expected []runner.StageStatus) {
	t.Helper()
	statuses := collectStageStatuses(sequence, stage)
	if len(statuses) != len(expected) {
		t.Fatalf("stage %s statuses length mismatch: got %d want %d", stage, len(statuses), len(expected))
	}
	for i, status := range statuses {
		if status != expected[i] {
			t.Fatalf("stage %s status %d mismatch: got %s want %s", stage, i, status, expected[i])
		}
	}
}

type recordingEvents struct {
    nextTicket     string
    invalidTicket  bool
    manifest       contracts.ManifestReference
    claimedTickets []string
    checkpoints    []contracts.WorkflowCheckpoint
	artifacts      []contracts.WorkflowArtifact
	mu             sync.Mutex
}

func (r *recordingEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	if ticketID == "" {
		ticketID = r.nextTicket
	}
	r.claimedTickets = append(r.claimedTickets, ticketID)
	if r.invalidTicket {
		return contracts.WorkflowTicket{}, nil
	}
	ref := r.manifest
	if ref.Name == "" && ref.Version == "" {
		ref = contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
    return contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: ticketID, Manifest: ref}, nil
}

func (r *recordingEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkpoints = append(r.checkpoints, checkpoint)
	return nil
}

func (r *recordingEvents) PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	r.artifacts = append(r.artifacts, artifact)
	return nil
}
