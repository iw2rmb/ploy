package contracts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSubjectsForTenant(t *testing.T) {
	subjects := SubjectsForTenant("acme", "ticket-123")
	if subjects.TicketInbox != "grid.webhook.acme" {
		t.Fatalf("TicketInbox mismatch: %s", subjects.TicketInbox)
	}
	if subjects.CheckpointStream != "ploy.workflow.ticket-123.checkpoints" {
		t.Fatalf("CheckpointStream mismatch: %s", subjects.CheckpointStream)
	}
	if subjects.ArtifactStream != "ploy.artifact.ticket-123" {
		t.Fatalf("ArtifactStream mismatch: %s", subjects.ArtifactStream)
	}
	if subjects.StatusStream != "grid.status.ticket-123" {
		t.Fatalf("StatusStream mismatch: %s", subjects.StatusStream)
	}
}

func TestWorkflowTicketValidate(t *testing.T) {
	ticket := WorkflowTicket{}
	if err := ticket.Validate(); err == nil {
		t.Fatal("expected validation error for empty ticket")
	}

	valid := WorkflowTicket{SchemaVersion: SchemaVersion, TicketID: "ticket-123", Tenant: "acme"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid ticket, got %v", err)
	}
}

func TestWorkflowCheckpointValidateAndMarshal(t *testing.T) {
	empty := WorkflowCheckpoint{}
	if err := empty.Validate(); err == nil {
		t.Fatal("expected validation error for empty checkpoint")
	}

	cp := WorkflowCheckpoint{
		SchemaVersion: SchemaVersion,
		TicketID:      "ticket-123",
		Stage:         "mods",
		Status:        CheckpointStatusPending,
	}
	if err := cp.Validate(); err != nil {
		t.Fatalf("expected valid checkpoint, got %v", err)
	}

	payload, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if !strings.Contains(string(payload), SchemaVersion) {
		t.Fatalf("expected payload to contain schema version %q: %s", SchemaVersion, string(payload))
	}
	if cp.Subject() != "ploy.workflow.ticket-123.checkpoints" {
		t.Fatalf("unexpected subject: %s", cp.Subject())
	}
}

func TestInMemoryBusRecordsMessages(t *testing.T) {
	bus := NewInMemoryBus("acme")
	ticket, err := bus.ClaimTicket(context.Background(), "ticket-123")
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}
	if ticket.Tenant != "acme" {
		t.Fatalf("unexpected tenant: %s", ticket.Tenant)
	}
	if len(bus.ClaimedTickets) != 1 {
		t.Fatalf("expected claimed ticket to be recorded")
	}

	checkpoint := WorkflowCheckpoint{
		SchemaVersion: SchemaVersion,
		TicketID:      "ticket-123",
		Stage:         "ticket-claimed",
		Status:        CheckpointStatusClaimed,
	}
	if err := bus.PublishCheckpoint(context.Background(), checkpoint); err != nil {
		t.Fatalf("publish error: %v", err)
	}
	if len(bus.Checkpoints) != 1 {
		t.Fatalf("expected checkpoint to be recorded")
	}
}
