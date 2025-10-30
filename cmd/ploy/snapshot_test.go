package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
	server "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type fakeSnapshotRegistry struct {
	planReport    snapshots.PlanReport
	captureResult snapshots.CaptureResult
	planErr       error
	captureErr    error
}

func (f *fakeSnapshotRegistry) Plan(ctx context.Context, name string) (snapshots.PlanReport, error) {
	return f.planReport, f.planErr
}

func (f *fakeSnapshotRegistry) Capture(ctx context.Context, name string, opts snapshots.CaptureOptions) (snapshots.CaptureResult, error) {
	return f.captureResult, f.captureErr
}

func TestHandleSnapshotPlanPrintsSummary(t *testing.T) {
	buf := &bytes.Buffer{}
	prevLoader := snapshotRegistryLoader
	prevDir := snapshotConfigDir
	defer func() {
		snapshotRegistryLoader = prevLoader
		snapshotConfigDir = prevDir
	}()

	report := snapshots.PlanReport{
		SnapshotName: "dev-db",
		Engine:       "postgres",
		Stripping:    snapshots.RuleSummary{Total: 1, Tables: map[string]int{"users": 1}},
		Masking:      snapshots.RuleSummary{Total: 2, Tables: map[string]int{"users": 2}},
		Synthetic:    snapshots.RuleSummary{Total: 1, Tables: map[string]int{"orders": 1}},
		Highlights:   []string{"mask users.email -> hash"},
	}

	snapshotRegistryLoader = func(dir string) (snapshotRegistry, error) {
		return &fakeSnapshotRegistry{planReport: report}, nil
	}
	snapshotConfigDir = "ignored"

	err := handleSnapshot([]string{"plan", "--snapshot", "dev-db"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	for _, fragment := range []string{"Snapshot: dev-db", "Engine: postgres", "Mask Rules: 2"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

func TestHandleSnapshotCapturePrintsResult(t *testing.T) {
	buf := &bytes.Buffer{}
	prevLoader := snapshotRegistryLoader
	prevDir := snapshotConfigDir
	defer func() {
		snapshotRegistryLoader = prevLoader
		snapshotConfigDir = prevDir
	}()

	result := snapshots.CaptureResult{
		ArtifactCID: "cid-dev",
		Fingerprint: "fp-123",
		Metadata: snapshots.SnapshotMetadata{
			SnapshotName: "dev-db",
        // tenant removed
			TicketID:     "ticket-123",
		},
	}

	snapshotRegistryLoader = func(dir string) (snapshotRegistry, error) {
		return &fakeSnapshotRegistry{captureResult: result}, nil
	}
	snapshotConfigDir = "ignored"

err := handleSnapshot([]string{"capture", "--snapshot", "dev-db", "--ticket", "ticket-123"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	for _, fragment := range []string{"Artifact CID: cid-dev", "Fingerprint: fp-123"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

func TestHandleSnapshotCaptureUsesIPFSGatewayWhenConfigured(t *testing.T) {
	buf := &bytes.Buffer{}
	serverCalled := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&serverCalled, 1)
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v0/add" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		defer func() { _ = file.Close() }()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read artifact body: %v", err)
		}
		if !strings.Contains(string(body), "users") {
			t.Fatalf("expected artifact body to contain users table, got %q", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Hash":"bafyrehandledcid","Name":"dev-db","Size":"42"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "dev-db.json")
	fixture := `{"users":[{"id":"1","email":"alice@example.com"}]}`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	specContent := `name = "dev-db"
description = "Development database"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "dev-db.json"
`
	specPath := filepath.Join(dir, "dev-db.toml")
	if err := os.WriteFile(specPath, []byte(specContent), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	prevDir := snapshotConfigDir
	snapshotConfigDir = dir
	defer func() { snapshotConfigDir = prevDir }()

	// Provide IPFS gateway via env override.
	os.Setenv("PLOY_IPFS_GATEWAY", server.URL)
	t.Cleanup(func() { os.Unsetenv("PLOY_IPFS_GATEWAY") })

err := handleSnapshot([]string{"capture", "--snapshot", "dev-db", "--ticket", "ticket-42"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&serverCalled) == 0 {
		t.Fatal("expected IPFS gateway to be invoked")
	}
	output := buf.String()
	if !strings.Contains(output, "Artifact CID: bafyrehandledcid") {
		t.Fatalf("expected output to include IPFS CID, got %q", output)
	}
}

func TestHandleSnapshotCapturePublishesMetadataToJetStream(t *testing.T) {
	buf := &bytes.Buffer{}

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

	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "dev-db.json")
	fixture := `{"users":[{"id":"1","email":"alice@example.com"}]}`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	specContent := `name = "dev-db"
description = "Development database"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "dev-db.json"
`
	specPath := filepath.Join(dir, "dev-db.toml")
	if err := os.WriteFile(specPath, []byte(specContent), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	prevDir := snapshotConfigDir
	snapshotConfigDir = dir
	t.Cleanup(func() { snapshotConfigDir = prevDir })

	// Provide JetStream URL via env override so loader wires metadata publishing.
	os.Setenv("PLOY_JETSTREAM_URL", srv.ClientURL())
	t.Cleanup(func() { os.Unsetenv("PLOY_JETSTREAM_URL") })

err = handleSnapshot([]string{"capture", "--snapshot", "dev-db", "--ticket", "ticket-77"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := js.GetMsg("PLOY_ARTIFACT", 1)
	if err != nil {
		t.Fatalf("get metadata msg: %v", err)
	}
	if msg.Subject != "ploy.artifact.ticket-77" {
		t.Fatalf("unexpected metadata subject: %s", msg.Subject)
	}

    var envelope struct { SchemaVersion string `json:"schema_version"`; SnapshotName string `json:"snapshot_name"`; ArtifactCID string `json:"artifact_cid"`; TicketID string `json:"ticket_id"`; CapturedAt string `json:"captured_at"` }
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		t.Fatalf("decode metadata envelope: %v", err)
	}
	if envelope.SchemaVersion != contracts.SchemaVersion {
		t.Fatalf("unexpected schema version: %s", envelope.SchemaVersion)
	}
	if envelope.SnapshotName != "dev-db" {
		t.Fatalf("snapshot mismatch: %s", envelope.SnapshotName)
	}
if envelope.TicketID != "ticket-77" {
    t.Fatalf("ticket mismatch: %s", envelope.TicketID)
}
	if envelope.ArtifactCID == "" {
		t.Fatalf("expected artifact cid in envelope")
	}
	if envelope.CapturedAt == "" {
		t.Fatalf("expected captured_at in envelope")
	}
}

func TestHandleSnapshotRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleSnapshot(nil, buf)
	if err == nil {
		t.Fatal("expected error for missing snapshot subcommand")
	}
	if !strings.Contains(buf.String(), "Usage: ploy snapshot") {
		t.Fatalf("expected snapshot usage, got %q", buf.String())
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
