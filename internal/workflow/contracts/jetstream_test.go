package contracts

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	server "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func TestJetStreamClientClaimTicket(t *testing.T) {
	srv := runJetStreamServer(t)
	defer srv.Shutdown()

	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	defer func() { _ = conn.Drain() }()

	js, err := conn.JetStream()
	if err != nil {
		t.Fatalf("jetstream context: %v", err)
	}

	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     "GRID_WEBHOOK",
		Subjects: []string{"grid.webhook.*"},
	}); err != nil {
		t.Fatalf("add webhook stream: %v", err)
	}

	ticket := WorkflowTicket{
		SchemaVersion: SchemaVersion,
		TicketID:      "ticket-123",
		Tenant:        "acme",
		Manifest:      ManifestReference{Name: "smoke", Version: "2025-09-26"},
	}
	payload, err := json.Marshal(ticket)
	if err != nil {
		t.Fatalf("marshal ticket: %v", err)
	}
	if _, err := js.Publish("grid.webhook.acme", payload); err != nil {
		t.Fatalf("publish ticket: %v", err)
	}

	client, err := NewJetStreamClient(JetStreamOptions{URL: srv.ClientURL(), Tenant: "acme"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	claimed, err := client.ClaimTicket(context.Background(), "")
	if err != nil {
		t.Fatalf("claim ticket: %v", err)
	}
	if claimed.TicketID != ticket.TicketID {
		t.Fatalf("expected ticket %s, got %s", ticket.TicketID, claimed.TicketID)
	}
	if claimed.Manifest.Name != ticket.Manifest.Name || claimed.Manifest.Version != ticket.Manifest.Version {
		t.Fatalf("manifest mismatch: %#v vs %#v", claimed.Manifest, ticket.Manifest)
	}
}

func TestJetStreamClientPublishCheckpoint(t *testing.T) {
	srv := runJetStreamServer(t)
	defer srv.Shutdown()

	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	defer func() { _ = conn.Drain() }()

	js, err := conn.JetStream()
	if err != nil {
		t.Fatalf("jetstream context: %v", err)
	}

	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     "PLOY_WORKFLOW",
		Subjects: []string{"ploy.workflow.*.checkpoints"},
	}); err != nil {
		t.Fatalf("add checkpoint stream: %v", err)
	}

	client, err := NewJetStreamClient(JetStreamOptions{URL: srv.ClientURL(), Tenant: "acme"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	checkpoint := WorkflowCheckpoint{
		SchemaVersion: SchemaVersion,
		TicketID:      "ticket-123",
		Stage:         "mods",
		Status:        CheckpointStatusRunning,
		CacheKey:      "node-wasm/node-wasm@manifest=2025-09-26@aster=plan",
		StageMetadata: &CheckpointStage{
			Name:     "mods",
			Kind:     "mods",
			Lane:     "node-wasm",
			Manifest: ManifestReference{Name: "smoke", Version: "2025-09-26"},
		},
		Artifacts: []CheckpointArtifact{{
			Name:        "mods-plan",
			ArtifactCID: "cid-mods-plan",
		}},
	}
	if err := client.PublishCheckpoint(context.Background(), checkpoint); err != nil {
		t.Fatalf("publish checkpoint: %v", err)
	}

	msg, err := js.GetMsg("PLOY_WORKFLOW", 1)
	if err != nil {
		t.Fatalf("get msg: %v", err)
	}
	if msg.Subject != checkpoint.Subject() {
		t.Fatalf("unexpected subject %s", msg.Subject)
	}
	var stored WorkflowCheckpoint
	if err := json.Unmarshal(msg.Data, &stored); err != nil {
		t.Fatalf("unmarshal checkpoint: %v", err)
	}
	if stored.TicketID != checkpoint.TicketID || stored.Stage != checkpoint.Stage {
		t.Fatalf("stored checkpoint mismatch: %#v", stored)
	}
	if stored.StageMetadata == nil || stored.StageMetadata.Lane != "node-wasm" {
		t.Fatalf("expected stage metadata in stored checkpoint: %#v", stored.StageMetadata)
	}
	if len(stored.Artifacts) != 1 || stored.Artifacts[0].ArtifactCID != "cid-mods-plan" {
		t.Fatalf("expected artifacts in stored checkpoint: %#v", stored.Artifacts)
	}
}

func TestJetStreamClientPublishArtifact(t *testing.T) {
	srv := runJetStreamServer(t)
	defer srv.Shutdown()

	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	defer func() { _ = conn.Drain() }()

	js, err := conn.JetStream()
	if err != nil {
		t.Fatalf("jetstream context: %v", err)
	}

	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     "PLOY_ARTIFACT",
		Subjects: []string{"ploy.artifact.*"},
	}); err != nil {
		t.Fatalf("add artifact stream: %v", err)
	}

	client, err := NewJetStreamClient(JetStreamOptions{URL: srv.ClientURL(), Tenant: "acme"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	envelope := WorkflowArtifact{
		SchemaVersion: SchemaVersion,
		TicketID:      "ticket-123",
		Stage:         "mods",
		CacheKey:      "node-wasm/node-wasm@manifest=2025-09-26@aster=plan",
		StageMetadata: &CheckpointStage{
			Name:     "mods",
			Kind:     "mods",
			Lane:     "node-wasm",
			Manifest: ManifestReference{Name: "smoke", Version: "2025-09-26"},
		},
		Artifact: CheckpointArtifact{
			Name:        "mods-plan",
			ArtifactCID: "cid-mods-plan",
			Digest:      "sha256:modsplan",
			MediaType:   "application/tar+zst",
		},
	}
	if err := client.PublishArtifact(context.Background(), envelope); err != nil {
		t.Fatalf("publish artifact: %v", err)
	}

	msg, err := js.GetMsg("PLOY_ARTIFACT", 1)
	if err != nil {
		t.Fatalf("get artifact msg: %v", err)
	}
	if msg.Subject != envelope.Subject() {
		t.Fatalf("unexpected artifact subject %s", msg.Subject)
	}
	var stored WorkflowArtifact
	if err := json.Unmarshal(msg.Data, &stored); err != nil {
		t.Fatalf("unmarshal artifact: %v", err)
	}
	if stored.TicketID != envelope.TicketID || stored.Stage != envelope.Stage {
		t.Fatalf("artifact mismatch: %#v", stored)
	}
	if stored.Artifact.ArtifactCID != "cid-mods-plan" {
		t.Fatalf("expected artifact CID, got %#v", stored.Artifact)
	}
}

func runJetStreamServer(t *testing.T) *server.Server {
	t.Helper()

	opts := &server.Options{
		JetStream: true,
		Host:      "127.0.0.1",
		Port:      -1,
		StoreDir:  t.TempDir(),
	}

	srv, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	go srv.Start()

	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatalf("nats server not ready")
	}

	return srv
}
