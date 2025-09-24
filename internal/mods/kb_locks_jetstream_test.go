package mods

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	server "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func runTestJetStream(t *testing.T) (*server.Server, string) {
	t.Helper()

	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  filepath.Join(t.TempDir(), "nats"),
	}

	srv, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create nats server: %v", err)
	}

	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatalf("nats server not ready in time")
	}

	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})

	return srv, srv.ClientURL()
}

func setJetStreamEnv(t *testing.T, url string) {
	t.Helper()

	t.Setenv("PLOY_JETSTREAM_URL", url)
	t.Setenv("NATS_ADDR", url)
	t.Setenv("PLOY_JETSTREAM_CREDS", "")
	t.Setenv("PLOY_JETSTREAM_USER", "")
	t.Setenv("PLOY_JETSTREAM_PASSWORD", "")
}

func TestJetstreamKBLockManager_AcquirePersistsMetadata(t *testing.T) {
	_, url := runTestJetStream(t)
	setJetStreamEnv(t, url)

	mgr, err := NewJetstreamKBLockManager()
	if err != nil {
		t.Fatalf("failed to create jetstream lock manager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	ctx := context.Background()
	lock, err := mgr.AcquireLock(ctx, "tenant/app/mod", 3*time.Second)
	if err != nil {
		t.Fatalf("expected first acquisition to succeed: %v", err)
	}

	if lock.Backend != "jetstream" {
		t.Fatalf("expected backend jetstream, got %q", lock.Backend)
	}

	if lock.Key != "writers/tenant/app/mod" {
		t.Fatalf("unexpected lock key: %s", lock.Key)
	}

	if lock.Revision == 0 {
		t.Fatal("expected lock revision to be populated")
	}

	if lock.LeaseExpiresAt.IsZero() {
		t.Fatal("expected lease expiry timestamp to be set")
	}

	if time.Until(lock.LeaseExpiresAt) > 3*time.Second+1*time.Second || time.Until(lock.LeaseExpiresAt) < 1*time.Second {
		t.Fatalf("unexpected lease expiry window: %v", lock.LeaseExpiresAt)
	}

	entry, err := mgr.bucket.Get("writers/tenant/app/mod")
	if err != nil {
		t.Fatalf("expected kv entry to exist: %v", err)
	}

	var stored map[string]interface{}
	if err := json.Unmarshal(entry.Value(), &stored); err != nil {
		t.Fatalf("failed to decode lock record: %v", err)
	}

	if entry.Revision() != lock.Revision {
		t.Fatalf("expected revision %d to match entry revision %d", lock.Revision, entry.Revision())
	}

	if owner, ok := stored["owner"].(string); !ok || owner == "" {
		t.Fatalf("expected stored owner to be populated, got %v", stored["owner"])
	}

	if expires, ok := stored["lease_expires_at"].(string); !ok || expires == "" {
		t.Fatalf("expected lease_expires_at timestamp, got %v", stored["lease_expires_at"])
	}

	if _, err := mgr.AcquireLock(ctx, "tenant/app/mod", 3*time.Second); err == nil {
		t.Fatal("expected contention error when acquiring held lock")
	}
}

func TestJetstreamKBLockManager_PublishesLifecycleEvents(t *testing.T) {
	_, url := runTestJetStream(t)
	setJetStreamEnv(t, url)

	conn, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("failed to create nats connection: %v", err)
	}
	defer conn.Close()

	js, err := conn.JetStream()
	if err != nil {
		t.Fatalf("failed to get jetstream context: %v", err)
	}

	mgr, err := NewJetstreamKBLockManager()
	if err != nil {
		t.Fatalf("failed to create jetstream lock manager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	if _, err := js.StreamInfo("mods_kb_lock_events"); err != nil {
		t.Fatalf("expected lock event stream to be created, got error: %v", err)
	}

	acquiredSub, err := js.SubscribeSync("mods.kb.lock.acquired.*")
	if err != nil {
		t.Fatalf("failed to subscribe to acquired events: %v", err)
	}
	releasedSub, err := js.SubscribeSync("mods.kb.lock.released.*")
	if err != nil {
		t.Fatalf("failed to subscribe to released events: %v", err)
	}

	ctx := context.Background()
	lock, err := mgr.AcquireLock(ctx, "tenant/app/mod", 2*time.Second)
	if err != nil {
		t.Fatalf("expected acquisition success: %v", err)
	}

	msg, err := acquiredSub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("expected acquisition event: %v", err)
	}
	validateLockEvent(t, msg, "acquired", "tenant/app/mod", lock.Revision)

	if err := mgr.ReleaseLock(ctx, lock); err != nil {
		t.Fatalf("expected release success: %v", err)
	}

	msg, err = releasedSub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("expected release event: %v", err)
	}
	validateLockEvent(t, msg, "released", "tenant/app/mod", lock.Revision)
}

func validateLockEvent(t *testing.T, msg *nats.Msg, event, kbID string, revision uint64) {
	t.Helper()

	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("failed to decode event payload: %v", err)
	}

	if payload["event"] != event {
		t.Fatalf("expected event %q, got %v", event, payload["event"])
	}

	if payload["kb_id"] != kbID {
		t.Fatalf("expected kb_id %q, got %v", kbID, payload["kb_id"])
	}

	if gotRevision, ok := payload["revision"].(float64); !ok || uint64(gotRevision) != revision {
		t.Fatalf("expected revision %d, got %v", revision, payload["revision"])
	}

	if _, ok := payload["owner"].(string); !ok {
		t.Fatalf("expected owner string in payload, got %v", payload["owner"])
	}

	if _, ok := payload["timestamp"].(string); !ok {
		t.Fatalf("expected timestamp string in payload, got %v", payload["timestamp"])
	}
}
