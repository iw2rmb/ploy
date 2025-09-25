package runner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

type stubEvents struct {
	claimedTicket string
	checkpoints   []contracts.WorkflowCheckpoint
}

func (s *stubEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	s.claimedTicket = ticketID
	return contracts.WorkflowTicket{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      ticketID,
		Tenant:        "acme",
	}, nil
}

func (s *stubEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	s.checkpoints = append(s.checkpoints, checkpoint)
	return nil
}

func TestRunRequiresTicket(t *testing.T) {
	opts := runner.Options{Events: &stubEvents{}}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrTicketRequired) {
		t.Fatalf("expected ErrTicketRequired, got %v", err)
	}
}

func TestRunRequiresEventsClient(t *testing.T) {
	opts := runner.Options{Ticket: "ticket-123"}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrEventsClientRequired) {
		t.Fatalf("expected ErrEventsClientRequired, got %v", err)
	}
}

func TestRunPublishesInitialCheckpoint(t *testing.T) {
	events := &stubEvents{}
	opts := runner.Options{Ticket: "ticket-123", Events: events}
	err := runner.Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if events.claimedTicket != "ticket-123" {
		t.Fatalf("ticket not claimed: %s", events.claimedTicket)
	}
	if len(events.checkpoints) != 1 {
		t.Fatalf("expected one checkpoint, got %d", len(events.checkpoints))
	}
	cp := events.checkpoints[0]
	if cp.Status != contracts.CheckpointStatusClaimed {
		t.Fatalf("expected claimed checkpoint, got %s", cp.Status)
	}
	if cp.TicketID != "ticket-123" {
		t.Fatalf("unexpected ticket id: %s", cp.TicketID)
	}
}
