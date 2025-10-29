package hydration

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

// TestPolicyEngineEnforcement emits enforcement decisions when hard limits are exceeded.
func TestPolicyEngineEnforcement(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e, client := newTestEtcd(t)
	t.Cleanup(func() {
		client.Close()
		e.Close()
	})

	clock := func() time.Time { return time.Date(2025, 10, 28, 21, 20, 0, 0, time.UTC) }
	store, err := NewPolicyStore(client, PolicyStoreOptions{Clock: clock})
	if err != nil {
		t.Fatalf("new policy store: %v", err)
	}
	index, err := NewIndex(client, IndexOptions{Clock: clock})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}

	policy := GlobalPolicy{
		ID:          "repo",
		Description: "default",
		Scope: PolicyScope{
			RepoPrefixes: []string{"https://git.example.com/org/"},
		},
		Window: QuotaWindow{
			PinnedBytes: LimitBytes{Hard: 50 << 20},
			Snapshots:   LimitCount{Hard: 10},
			Replicas:    LimitCount{Hard: 3},
		},
	}
	if _, err := store.SavePolicy(ctx, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	snapshots := []SnapshotRecord{
		{
			RepoURL:  "https://git.example.com/org/repo.git",
			Revision: "abc123",
			TicketID: "mod-a",
			Bundle: scheduler.BundleRecord{
				CID:       "cid-a",
				Size:      40 << 20,
				TTL:       scheduler.HydrationSnapshotTTL,
				ExpiresAt: clock().Add(24 * time.Hour).Format(time.RFC3339Nano),
			},
			Replication: ReplicationPolicy{Min: 1, Max: 5},
		},
		{
			RepoURL:  "https://git.example.com/org/repo.git",
			Revision: "def456",
			TicketID: "mod-b",
			Bundle: scheduler.BundleRecord{
				CID:       "cid-b",
				Size:      20 << 20,
				TTL:       scheduler.HydrationSnapshotTTL,
				ExpiresAt: clock().Add(24 * time.Hour).Format(time.RFC3339Nano),
			},
			Replication: ReplicationPolicy{Min: 1, Max: 2},
		},
	}
	for _, record := range snapshots {
		if _, err := index.UpsertSnapshot(ctx, record); err != nil {
			t.Fatalf("upsert snapshot: %v", err)
		}
	}

	engine, err := NewPolicyEngine(PolicyEngineOptions{
		Store: store,
		Index: index,
		Clock: clock,
	})
	if err != nil {
		t.Fatalf("new policy engine: %v", err)
	}
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}

	engine.Trigger()

	decisions := collectDecisions(t, engine.Decisions(), 2)
	cancel()

	vary := map[PolicyDecisionAction]bool{}
	for _, decision := range decisions {
		vary[decision.Action] = true
	}
	if !vary[PolicyActionReduceReplication] {
		t.Fatalf("expected reduce replication decision, got %+v", decisions)
	}
	if !vary[PolicyActionEvictSnapshot] {
		t.Fatalf("expected eviction decision, got %+v", decisions)
	}
}

// TestPolicyEngineWarn emits warnings when soft limits trip.
func TestPolicyEngineWarn(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e, client := newTestEtcd(t)
	t.Cleanup(func() {
		client.Close()
		e.Close()
	})

	clock := func() time.Time { return time.Date(2025, 10, 28, 21, 30, 0, 0, time.UTC) }
	store, _ := NewPolicyStore(client, PolicyStoreOptions{Clock: clock})
	index, _ := NewIndex(client, IndexOptions{Clock: clock})

	_, _ = store.SavePolicy(ctx, GlobalPolicy{
		ID: "warn",
		Scope: PolicyScope{
			RepoPrefixes: []string{"https://git.example.com/"},
		},
		Window: QuotaWindow{
			Snapshots: LimitCount{Soft: 1, Hard: 5},
		},
	})

	_, _ = index.UpsertSnapshot(ctx, SnapshotRecord{
		RepoURL:  "https://git.example.com/repo.git",
		Revision: "main",
		TicketID: "mod-soft",
		Bundle: scheduler.BundleRecord{
			CID:       "cid-soft",
			TTL:       scheduler.HydrationSnapshotTTL,
			ExpiresAt: clock().Add(24 * time.Hour).Format(time.RFC3339Nano),
		},
	})
	_, _ = index.UpsertSnapshot(ctx, SnapshotRecord{
		RepoURL:  "https://git.example.com/repo.git",
		Revision: "feature",
		TicketID: "mod-soft-2",
		Bundle: scheduler.BundleRecord{
			CID:       "cid-soft-2",
			TTL:       scheduler.HydrationSnapshotTTL,
			ExpiresAt: clock().Add(24 * time.Hour).Format(time.RFC3339Nano),
		},
	})

	engine, err := NewPolicyEngine(PolicyEngineOptions{Store: store, Index: index, Clock: clock})
	if err != nil {
		t.Fatalf("new policy engine: %v", err)
	}
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}

	engine.Trigger()
	decisions := collectDecisions(t, engine.Decisions(), 1)
	cancel()

	if len(decisions) == 0 {
		t.Fatalf("expected warning decision")
	}
	if decisions[0].Level != PolicyDecisionLevelWarn {
		t.Fatalf("expected warn level, got %s", decisions[0].Level)
	}
	if decisions[0].Action != PolicyActionWarn {
		t.Fatalf("expected warn action, got %s", decisions[0].Action)
	}
}

func collectDecisions(t *testing.T, ch <-chan PolicyDecision, want int) []PolicyDecision {
	t.Helper()
	deadline := time.After(2 * time.Second)
	out := make([]PolicyDecision, 0, want)
	for len(out) < want {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d decisions, got %d", want, len(out))
		case decision, ok := <-ch:
			if !ok {
				t.Fatalf("decision channel closed prematurely")
			}
			out = append(out, decision)
		}
	}
	return out
}
