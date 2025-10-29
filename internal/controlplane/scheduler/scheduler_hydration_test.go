package scheduler_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

func TestCompleteJobAddsHydrationSnapshotBundle(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	completedAt := time.Date(2025, 10, 28, 12, 0, 0, 0, time.UTC)
	opts := defaultOptions()
	opts.Now = func() time.Time { return completedAt }

	sched := mustNewScheduler(t, client, opts)
	defer func() {
		_ = sched.Close()
	}()

	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-hydrate",
		StepID:      "mods-plan",
		Priority:    "default",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	if _, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "node-hydrate"}); err != nil {
		t.Fatalf("claim job: %v", err)
	}

	size := int64(4096)
	result, err := sched.CompleteJob(ctx, scheduler.CompleteRequest{
		JobID:  job.ID,
		Ticket: job.Ticket,
		NodeID: "node-hydrate",
		State:  scheduler.JobStateSucceeded,
		Artifacts: map[string]string{
			scheduler.HydrationSnapshotCIDKey:    "bafy-snapshot",
			scheduler.HydrationSnapshotDigestKey: "sha256:snapshot",
			scheduler.HydrationSnapshotSizeKey:   strconv.FormatInt(size, 10),
		},
	})
	if err != nil {
		t.Fatalf("complete job: %v", err)
	}

	bundle, ok := result.Bundles[scheduler.HydrationSnapshotBundleKey]
	if !ok {
		t.Fatalf("expected hydration snapshot bundle in job result")
	}
	if bundle.CID != "bafy-snapshot" {
		t.Fatalf("unexpected cid: %s", bundle.CID)
	}
	if bundle.Digest != "sha256:snapshot" {
		t.Fatalf("unexpected digest: %s", bundle.Digest)
	}
	if bundle.Size != size {
		t.Fatalf("unexpected size: %d", bundle.Size)
	}
	if bundle.TTL != scheduler.HydrationSnapshotTTL {
		t.Fatalf("expected ttl %s, got %q", scheduler.HydrationSnapshotTTL, bundle.TTL)
	}
	expectedExpiry := ""
	if duration, err := time.ParseDuration(scheduler.HydrationSnapshotTTL); err == nil && duration > 0 {
		expectedExpiry = completedAt.Add(duration).UTC().Format(time.RFC3339Nano)
	}
	if bundle.ExpiresAt != expectedExpiry {
		t.Fatalf("expected expiry %s, got %s", expectedExpiry, bundle.ExpiresAt)
	}
	if !bundle.Retained {
		t.Fatalf("expected hydration bundle retained flag true")
	}
	if result.Artifacts[scheduler.HydrationSnapshotCIDKey] != "bafy-snapshot" {
		t.Fatalf("expected hydration artifact persisted on job")
	}
	if result.Artifacts[scheduler.HydrationSnapshotDigestKey] != "sha256:snapshot" {
		t.Fatalf("expected hydration digest stored")
	}
}
