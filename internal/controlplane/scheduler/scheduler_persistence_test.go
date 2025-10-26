package scheduler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

func TestSchedulerCompleteJobRecordsBundles(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	completedAt := time.Date(2025, 10, 22, 14, 0, 0, 0, time.UTC)
	opts := defaultOptions()
	opts.Now = func() time.Time { return completedAt }

	sched := mustNewScheduler(t, client, opts)
	defer func() { _ = sched.Close() }()

	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-bundle",
		StepID:      "logs",
		Priority:    "default",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	claim, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "worker-a"})
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}

	_, err = sched.CompleteJob(ctx, scheduler.CompleteRequest{
		JobID:  claim.Job.ID,
		Ticket: job.Ticket,
		NodeID: "worker-a",
		State:  scheduler.JobStateSucceeded,
		Bundles: map[string]scheduler.BundleRecord{
			"logs": {
				CID:      "bafy-log",
				Digest:   "sha256:bundle",
				Size:     4096,
				Retained: true,
				TTL:      "24h",
			},
		},
	})
	if err != nil {
		t.Fatalf("complete job: %v", err)
	}

	stored, err := sched.GetJob(ctx, job.Ticket, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if stored.Bundles == nil {
		t.Fatalf("expected bundles recorded")
	}
	logBundle, ok := stored.Bundles["logs"]
	if !ok {
		t.Fatalf("expected logs bundle present")
	}
	if logBundle.CID != "bafy-log" {
		t.Fatalf("unexpected cid: %s", logBundle.CID)
	}
	if logBundle.Digest != "sha256:bundle" {
		t.Fatalf("unexpected digest: %s", logBundle.Digest)
	}
	if logBundle.Size != 4096 {
		t.Fatalf("unexpected size: %d", logBundle.Size)
	}
	if !logBundle.Retained {
		t.Fatalf("expected retained flag true")
	}
	if logBundle.TTL != "24h" {
		t.Fatalf("unexpected ttl: %s", logBundle.TTL)
	}
	wantExpires := completedAt.Add(24 * time.Hour).UTC().Format(time.RFC3339Nano)
	if logBundle.ExpiresAt != wantExpires {
		t.Fatalf("unexpected expires_at: %s want %s", logBundle.ExpiresAt, wantExpires)
	}
}

func TestSchedulerRetentionSummaryUpdatesGCExpiry(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	completedAt := time.Date(2025, 10, 22, 16, 0, 0, 0, time.UTC)
	opts := defaultOptions()
	opts.Now = func() time.Time { return completedAt }

	sched := mustNewScheduler(t, client, opts)
	defer func() { _ = sched.Close() }()

	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-retention",
		StepID:      "logs",
		Priority:    "default",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	claim, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "worker-retention"})
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}

	result, err := sched.CompleteJob(ctx, scheduler.CompleteRequest{
		JobID:  claim.Job.ID,
		Ticket: job.Ticket,
		NodeID: "worker-retention",
		State:  scheduler.JobStateSucceeded,
		Bundles: map[string]scheduler.BundleRecord{
			"logs": {
				CID:      "bafy-retained",
				Digest:   "sha256:bundle",
				Size:     2048,
				Retained: true,
				TTL:      "72h",
			},
		},
	})
	if err != nil {
		t.Fatalf("complete job: %v", err)
	}

	if result.Retention == nil {
		t.Fatalf("expected retention summary on completion")
	}
	if result.Retention.Bundle != "logs" {
		t.Fatalf("unexpected retention bundle: %s", result.Retention.Bundle)
	}
	if result.Retention.BundleCID != "bafy-retained" {
		t.Fatalf("unexpected retention cid: %s", result.Retention.BundleCID)
	}
	if !result.Retention.Retained {
		t.Fatalf("expected retained flag true")
	}
	if result.Retention.TTL != "72h" {
		t.Fatalf("unexpected retention ttl: %s", result.Retention.TTL)
	}
	wantExpires := completedAt.Add(72 * time.Hour).UTC().Format(time.RFC3339Nano)
	if result.Retention.ExpiresAt != wantExpires {
		t.Fatalf("unexpected retention expires_at: %s want %s", result.Retention.ExpiresAt, wantExpires)
	}
	if result.Retention.Inspection {
		t.Fatalf("expected inspection hint false")
	}

	stored, err := sched.GetJob(ctx, job.Ticket, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if stored.Retention == nil {
		t.Fatalf("expected persisted retention summary")
	}
	if stored.Retention.Bundle != "logs" {
		t.Fatalf("unexpected persisted bundle: %s", stored.Retention.Bundle)
	}
	if stored.Retention.BundleCID != "bafy-retained" {
		t.Fatalf("unexpected persisted cid: %s", stored.Retention.BundleCID)
	}
	if stored.Retention.ExpiresAt != wantExpires {
		t.Fatalf("unexpected persisted expires_at: %s want %s", stored.Retention.ExpiresAt, wantExpires)
	}

	gcKey := fmt.Sprintf("gc/jobs/%s", job.ID)
	resp, err := client.Get(ctx, gcKey)
	if err != nil {
		t.Fatalf("get gc entry: %v", err)
	}
	if len(resp.Kvs) != 1 {
		t.Fatalf("expected gc marker, got %d entries", len(resp.Kvs))
	}
	var gcPayload struct {
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(resp.Kvs[0].Value, &gcPayload); err != nil {
		t.Fatalf("decode gc payload: %v", err)
	}
	if gcPayload.ExpiresAt != wantExpires {
		t.Fatalf("unexpected gc expires_at: %s want %s", gcPayload.ExpiresAt, wantExpires)
	}
}

func TestSchedulerGCWatcherSyncsExpiry(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	opts := defaultOptions()
	now := time.Date(2025, 10, 24, 13, 0, 0, 0, time.UTC)
	opts.Now = func() time.Time { return now }

	sched := mustNewScheduler(t, client, opts)
	defer func() { _ = sched.Close() }()

	nodeID := "node-gc"
	capacityJSON := `{"cpu_free": 5000, "mem_free": 12288, "heartbeat": "2025-10-24T12:59:30Z", "revision": 20}`
	if _, err := client.Put(ctx, opts.NodesPrefix+nodeID+"/capacity", capacityJSON); err != nil {
		t.Fatalf("put capacity: %v", err)
	}

	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-gc",
		StepID:      "archive",
		Priority:    "default",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	claim, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: nodeID})
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}

	if _, err := sched.CompleteJob(ctx, scheduler.CompleteRequest{
		JobID:  claim.Job.ID,
		Ticket: job.Ticket,
		NodeID: nodeID,
		State:  scheduler.JobStateSucceeded,
		Bundles: map[string]scheduler.BundleRecord{
			"logs": {
				CID:      "bafy-gc",
				Digest:   "sha256:gc",
				Size:     1024,
				Retained: true,
				TTL:      "24h",
			},
		},
	}); err != nil {
		t.Fatalf("complete job: %v", err)
	}

	gcKey := opts.GCPrefix + claim.Job.ID
	newExpiry := now.Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	payload, err := json.Marshal(map[string]any{
		"job_id":      claim.Job.ID,
		"ticket":      job.Ticket,
		"state":       string(scheduler.JobStateSucceeded),
		"final_state": string(scheduler.JobStateSucceeded),
		"expires_at":  newExpiry,
	})
	if err != nil {
		t.Fatalf("marshal gc payload: %v", err)
	}
	if _, err := client.Put(ctx, gcKey, string(payload)); err != nil {
		t.Fatalf("update gc marker: %v", err)
	}

	jobKey := opts.JobsPrefix + job.Ticket + "/jobs/" + job.ID
	waitForCondition(t, 5*time.Second, func() bool {
		doc, err := loadJobDocument(ctx, client, jobKey)
		if err != nil {
			return false
		}
		expiresAt, ok := doc["expires_at"].(string)
		if !ok || expiresAt != newExpiry {
			return false
		}
		retentionVal, ok := doc["retention"]
		if !ok {
			return false
		}
		retention, ok := retentionVal.(map[string]any)
		if !ok {
			return false
		}
		retentionExpiry, ok := retention["expires_at"].(string)
		if !ok || retentionExpiry != newExpiry {
			return false
		}
		return true
	})
}
