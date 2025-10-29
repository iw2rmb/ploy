package httpserver_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/hydration"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

// TestHydrationInspectAndTune exercises the hydration policy endpoints.
func TestHydrationInspectAndTune(t *testing.T) {
	t.Parallel()

	fixture := newModsServerFixture(t)
	index := fixture.hydrationIndex
	if index == nil {
		t.Fatalf("hydration index not initialised in fixture")
	}

	ticketID := "mod-hydration-api"
	if _, err := index.UpsertSnapshot(fixture.ctx, hydration.SnapshotRecord{
		RepoURL:  "https://git.example.com/org/repo.git",
		Revision: "cafebabe",
		TicketID: ticketID,
		Bundle: scheduler.BundleRecord{
			CID:       "bafy-hydration",
			Digest:    "sha256:1234",
			Size:      2048,
			TTL:       scheduler.HydrationSnapshotTTL,
			ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano),
			Retained:  true,
		},
		Replication: hydration.ReplicationPolicy{Min: 1, Max: 2},
		Sharing:     hydration.SharingPolicy{Enabled: true},
	}); err != nil {
		t.Fatalf("upsert snapshot: %v", err)
	}

	entry, ok, err := index.LookupSnapshot(fixture.ctx, hydration.LookupRequest{
		RepoURL:  "https://git.example.com/org/repo.git",
		Revision: "cafebabe",
	})
	if err != nil || !ok {
		t.Fatalf("lookup snapshot for usage: ok=%t err=%v", ok, err)
	}
	if _, err := fixture.policyStore.RecordUsage(fixture.ctx, "default", hydration.PolicyUsage{
		PolicyID:           "default",
		PinnedBytes:        entry.Bundle.Size,
		SnapshotCount:      1,
		ReplicaCount:       entry.Replication.Max,
		ActiveFingerprints: []string{entry.Fingerprint},
	}); err != nil {
		t.Fatalf("record usage: %v", err)
	}

	status := getJSON(t, fmt.Sprintf("%s/v1/mods/%s/hydration", fixture.server.URL, ticketID))
	policy, ok := status["hydration"].(map[string]any)
	if !ok {
		t.Fatalf("expected hydration block in response, got %+v", status)
	}
	if cid, _ := policy["shared_cid"].(string); cid != "bafy-hydration" {
		t.Fatalf("unexpected shared cid %q", cid)
	}
	if fib, _ := policy["replication_min"].(float64); int(fib) != 1 {
		t.Fatalf("unexpected replication min %v", fib)
	}
	global, ok := policy["global"].(map[string]any)
	if !ok {
		t.Fatalf("expected global policy block, got %+v", policy)
	}
	if id, _ := global["policy_id"].(string); id != "default" {
		t.Fatalf("unexpected global policy id %q", id)
	}
	pinned, ok := global["pinned_bytes"].(map[string]any)
	if !ok {
		t.Fatalf("expected pinned bytes usage, got %+v", global)
	}
	if used, _ := pinned["used"].(float64); int(used) != 2048 {
		t.Fatalf("unexpected pinned usage %v", used)
	}

	payload := map[string]any{
		"ttl":             "36h",
		"replication_min": 2,
		"replication_max": 4,
		"share":           false,
	}
	statusCode, body := patchJSONStatus(t, fmt.Sprintf("%s/v1/mods/%s/hydration", fixture.server.URL, ticketID), payload)
	if statusCode != 202 {
		t.Fatalf("expected patch status 202, got %d body=%v", statusCode, body)
	}

	entry, ok, err = index.LookupSnapshot(fixture.ctx, hydration.LookupRequest{
		RepoURL:  "https://git.example.com/org/repo.git",
		Revision: "cafebabe",
	})
	if err != nil {
		t.Fatalf("lookup snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("expected snapshot after patch")
	}
	if entry.Replication.Min != 2 || entry.Replication.Max != 4 {
		t.Fatalf("expected replication override, got %#v", entry.Replication)
	}
	if entry.Sharing.Enabled {
		t.Fatalf("expected sharing disabled after patch")
	}
	if entry.Bundle.TTL != "36h" {
		t.Fatalf("expected bundle ttl updated, got %q", entry.Bundle.TTL)
	}
}
