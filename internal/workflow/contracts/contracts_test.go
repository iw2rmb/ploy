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

	valid := WorkflowTicket{
		SchemaVersion: SchemaVersion,
		TicketID:      "ticket-123",
		Tenant:        "acme",
		Manifest:      ManifestReference{Name: "smoke", Version: "2025-09-26"},
	}
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
		CacheKey:      "node-wasm/cache@manifest=2025-09-26@aster=plan",
		StageMetadata: &CheckpointStage{
			Name:         "mods",
			Kind:         "mods",
			Lane:         "node-wasm",
			Dependencies: []string{},
			Manifest:     ManifestReference{Name: "smoke", Version: "2025-09-26"},
			Aster: CheckpointStageAster{
				Enabled: true,
				Toggles: []string{"plan"},
				Bundles: []CheckpointAsterBundle{{
					Stage:       "mods",
					Toggle:      "plan",
					BundleID:    "mods-plan",
					Digest:      "sha256:modsplan",
					ArtifactCID: "cid-mods-plan",
				}},
			},
		},
		Artifacts: []CheckpointArtifact{{
			Name:        "mods-plan-bundle",
			ArtifactCID: "cid-mods-plan",
			Digest:      "sha256:modsplan",
			MediaType:   "application/tar+zst",
		}},
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
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if _, ok := decoded["stage_metadata"].(map[string]any); !ok {
		t.Fatalf("expected stage metadata in payload: %v", decoded)
	}
	if artifacts, ok := decoded["artifacts"].([]any); !ok || len(artifacts) == 0 {
		t.Fatalf("expected artifacts in payload: %v", decoded)
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
	if ticket.Manifest.Name == "" || ticket.Manifest.Version == "" {
		t.Fatalf("expected manifest reference to be set, got %+v", ticket.Manifest)
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

func TestInMemoryBusAutoTicketFallback(t *testing.T) {
	bus := NewInMemoryBus("acme")
	bus.EnqueueTicket("queued-1")
	ticket, err := bus.ClaimTicket(context.Background(), "")
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}
	if ticket.TicketID != "queued-1" {
		t.Fatalf("expected queued ticket, got %s", ticket.TicketID)
	}
	if len(bus.ClaimedTickets) != 1 || bus.ClaimedTickets[0] != "queued-1" {
		t.Fatalf("unexpected claimed tickets: %v", bus.ClaimedTickets)
	}

	second, err := bus.ClaimTicket(context.Background(), "")
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}
	if second.TicketID == "" {
		t.Fatal("expected auto-generated ticket id")
	}
	if second.TicketID == "queued-1" {
		t.Fatal("expected different ticket id for auto fallback")
	}
	if len(bus.ClaimedTickets) != 2 {
		t.Fatalf("expected two claimed tickets, got %v", bus.ClaimedTickets)
	}
	if second.Manifest.Name == "" || second.Manifest.Version == "" {
		t.Fatalf("expected auto manifest assignment, got %+v", second.Manifest)
	}
}
