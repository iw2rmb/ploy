package snapshots

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	server "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func TestJetStreamMetadataPublisherPublishesEnvelope(t *testing.T) {
	srv := runJetStreamServer(t)
	t.Cleanup(func() { srv.Shutdown() })

	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	t.Cleanup(func() { _ = conn.Drain() })

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

	publisher, err := NewJetStreamMetadataPublisher(srv.ClientURL(), JetStreamMetadataOptions{})
	if err != nil {
		t.Fatalf("new metadata publisher: %v", err)
	}

    meta := SnapshotMetadata{SnapshotName: "dev-db", Description: "Development database", TicketID: "ticket-7", Engine: "postgres", DSN: "postgres://dev", Fingerprint: "fp-123", ArtifactCID: "cid-abc", CapturedAt: time.Date(2025, 9, 26, 12, 0, 0, 0, time.UTC), RuleCounts: RuleCounts{Strip:1, Mask:2, Synthetic:0}}

	if err := publisher.Publish(context.Background(), meta); err != nil {
		t.Fatalf("publish metadata: %v", err)
	}

	msg, err := js.GetMsg("PLOY_ARTIFACT", 1)
	if err != nil {
		t.Fatalf("get metadata msg: %v", err)
	}
	if msg.Subject != "ploy.artifact.ticket-7" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}

    var envelope struct { SchemaVersion string `json:"schema_version"`; SnapshotName string `json:"snapshot_name"`; ArtifactCID string `json:"artifact_cid"`; TicketID string `json:"ticket_id"`; Fingerprint string `json:"fingerprint"` }
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		t.Fatalf("decode metadata envelope: %v", err)
	}
	if envelope.SchemaVersion != contracts.SchemaVersion {
		t.Fatalf("unexpected schema version: %s", envelope.SchemaVersion)
	}
	if envelope.SnapshotName != meta.SnapshotName {
		t.Fatalf("snapshot name mismatch: %s", envelope.SnapshotName)
	}
	if envelope.ArtifactCID != meta.ArtifactCID {
		t.Fatalf("artifact cid mismatch: %s", envelope.ArtifactCID)
	}
    if envelope.TicketID != meta.TicketID { t.Fatalf("ticket mismatch: %s", envelope.TicketID) }
	if envelope.Fingerprint != meta.Fingerprint {
		t.Fatalf("fingerprint mismatch: %s", envelope.Fingerprint)
	}
}

func TestJetStreamMetadataPublisherValidatesInputs(t *testing.T) {
	if _, err := NewJetStreamMetadataPublisher("", JetStreamMetadataOptions{}); err == nil {
		t.Fatal("expected error for empty jetstream url")
	}

	srv := runJetStreamServer(t)
	t.Cleanup(func() { srv.Shutdown() })

	publisher, err := NewJetStreamMetadataPublisher(srv.ClientURL(), JetStreamMetadataOptions{})
	if err != nil {
		t.Fatalf("new metadata publisher: %v", err)
	}

	meta := SnapshotMetadata{
		SnapshotName: "dev-db",
		ArtifactCID:  "cid-abc",
	}

    if err := publisher.Publish(context.Background(), meta); err == nil {
        t.Fatal("expected error for missing ticket id")
    }

    meta.TicketID = "ticket-1"
    meta.ArtifactCID = ""
    if err := publisher.Publish(context.Background(), meta); err == nil {
        t.Fatal("expected error for missing artifact cid")
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
