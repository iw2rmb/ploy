package hydration

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestPolicyStoreSaveAndList ensures policies can be created and listed with defaults.
func TestPolicyStoreSaveAndList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	t.Cleanup(func() {
		client.Close()
		e.Close()
	})

	clock := func() time.Time { return time.Date(2025, 10, 28, 21, 0, 0, 0, time.UTC) }
	store, err := NewPolicyStore(client, PolicyStoreOptions{Clock: clock})
	if err != nil {
		t.Fatalf("new policy store: %v", err)
	}

	policy := GlobalPolicy{
		ID: "global",
		Scope: PolicyScope{
			RepoPrefixes: []string{"https://git.example.com/org/"},
		},
		Window: QuotaWindow{
			PinnedBytes: LimitBytes{Soft: 25 << 20, Hard: 50 << 20},
			Snapshots:   LimitCount{Hard: 10},
			Replicas:    LimitCount{Hard: 3},
		},
	}

	saved, err := store.SavePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("save policy: %v", err)
	}
	if saved.Version != 1 {
		t.Fatalf("expected version 1, got %d", saved.Version)
	}
	if saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps set")
	}

	listed, err := store.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("list policies: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(listed))
	}
	if listed[0].ID != "global" {
		t.Fatalf("unexpected policy id %q", listed[0].ID)
	}
	if listed[0].Window.PinnedBytes.Hard != 50<<20 {
		t.Fatalf("unexpected hard bytes %d", listed[0].Window.PinnedBytes.Hard)
	}
	if len(listed[0].Scope.RepoPrefixes) != 1 {
		t.Fatalf("expected repo scope preserved")
	}
}

// TestPolicyStoreVersionConflict verifies optimistic concurrency on updates.
func TestPolicyStoreVersionConflict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	t.Cleanup(func() {
		client.Close()
		e.Close()
	})

	store, err := NewPolicyStore(client, PolicyStoreOptions{})
	if err != nil {
		t.Fatalf("new policy store: %v", err)
	}

	policy := GlobalPolicy{
		ID: "global",
		Window: QuotaWindow{
			PinnedBytes: LimitBytes{Hard: 50 << 20},
			Snapshots:   LimitCount{Hard: 10},
			Replicas:    LimitCount{Hard: 3},
		},
	}

	saved, err := store.SavePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("initial save: %v", err)
	}

	// Attempt to save with stale version.
	saved.Window.PinnedBytes.Hard = 75 << 20
	saved.Version = 0
	if _, err := store.SavePolicy(ctx, saved); !errors.Is(err, ErrPolicyVersionConflict) {
		t.Fatalf("expected version conflict when version missing, got %v", err)
	}

	saved.Version = 99
	if _, err := store.SavePolicy(ctx, saved); !errors.Is(err, ErrPolicyVersionConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

// TestPolicyStoreRecordUsage ensures usage snapshots replace previous totals.
func TestPolicyStoreRecordUsage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	t.Cleanup(func() {
		client.Close()
		e.Close()
	})

	clock := func() time.Time { return time.Date(2025, 10, 28, 21, 5, 0, 0, time.UTC) }
	store, err := NewPolicyStore(client, PolicyStoreOptions{Clock: clock})
	if err != nil {
		t.Fatalf("new policy store: %v", err)
	}

	_, err = store.SavePolicy(ctx, GlobalPolicy{
		ID: "global",
		Window: QuotaWindow{
			PinnedBytes: LimitBytes{Hard: 50 << 20},
			Snapshots:   LimitCount{Hard: 10},
			Replicas:    LimitCount{Hard: 3},
		},
	})
	if err != nil {
		t.Fatalf("prime policy: %v", err)
	}

	usage := PolicyUsage{
		PolicyID:           "global",
		PinnedBytes:        40 << 20,
		SnapshotCount:      5,
		ReplicaCount:       12,
		ActiveFingerprints: []string{"a", "b", "a"},
	}

	updated, err := store.RecordUsage(ctx, "global", usage)
	if err != nil {
		t.Fatalf("record usage: %v", err)
	}
	if updated.Usage.PinnedBytes != 40<<20 {
		t.Fatalf("unexpected pinned bytes %d", updated.Usage.PinnedBytes)
	}
	if updated.Usage.SnapshotCount != 5 {
		t.Fatalf("unexpected snapshot count %d", updated.Usage.SnapshotCount)
	}
	if len(updated.Usage.ActiveFingerprints) != 2 {
		t.Fatalf("expected fingerprints deduplicated, got %v", updated.Usage.ActiveFingerprints)
	}
	if updated.Version < 2 {
		t.Fatalf("expected version increment, got %d", updated.Version)
	}
	if updated.Usage.UpdatedAt.IsZero() {
		t.Fatalf("expected usage timestamp set")
	}
}
