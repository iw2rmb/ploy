package scheduler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

func TestSchedulerSubmitAndClaimSingleJob(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	sched := newScheduler(t, client)
	defer func() {
		_ = sched.Close()
	}()

	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-123",
		StepID:      "plan",
		Priority:    "default",
		MaxAttempts: 3,
		Metadata:    map[string]string{"lane": "mods-plan"},
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	if job.State != scheduler.JobStateQueued {
		t.Fatalf("expected queued state, got %s", job.State)
	}

	var (
		wg      sync.WaitGroup
		claimMu sync.Mutex
		claims  []*scheduler.ClaimResult
		errs    []error
	)
	workers := []string{"node-a", "node-b"}
	wg.Add(len(workers))
	for _, nodeID := range workers {
		nodeID := nodeID
		go func() {
			defer wg.Done()
			res, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: nodeID})
			claimMu.Lock()
			defer claimMu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			claims = append(claims, res)
		}()
	}
	wg.Wait()

	if len(claims) != 1 {
		t.Fatalf("expected exactly one successful claim, got %d (errs=%d)", len(claims), len(errs))
	}
	if claims[0].Job.ID != job.ID {
		t.Fatalf("claimed job mismatch: %s vs %s", claims[0].Job.ID, job.ID)
	}
	if claims[0].Job.ClaimedBy != claims[0].NodeID {
		t.Fatalf("claim metadata lost: job claimed by %s, result node %s", claims[0].Job.ClaimedBy, claims[0].NodeID)
	}
	if claims[0].Job.State != scheduler.JobStateRunning {
		t.Fatalf("expected running state post-claim, got %s", claims[0].Job.State)
	}

	if _, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "node-c"}); err == nil {
		t.Fatalf("expected no jobs left to claim")
	}
}

func TestSchedulerLeaseExpiryRequeuesJob(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	opts := defaultOptions()
	opts.LeaseTTL = 2 * time.Second
	sched := mustNewScheduler(t, client, opts)
	defer func() {
		_ = sched.Close()
	}()

	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-456",
		StepID:      "rewrite",
		Priority:    "default",
		MaxAttempts: 2,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	claim, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "node-fail"})
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	if claim.Job.ID != job.ID {
		t.Fatalf("claim mismatch")
	}

	// Allow lease to expire without heartbeat.
	time.Sleep(opts.LeaseTTL + 150*time.Millisecond)

	// Wait for requeue.
	waitForCondition(t, opts.LeaseTTL+2*time.Second, func() bool {
		_, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "node-new"})
		return err == nil
	})
}

func TestSchedulerHeartbeatExtendsLease(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	opts := defaultOptions()
	opts.LeaseTTL = 2 * time.Second
	sched := mustNewScheduler(t, client, opts)
	defer func() {
		_ = sched.Close()
	}()

	if _, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-789",
		StepID:      "validate",
		Priority:    "default",
		MaxAttempts: 1,
	}); err != nil {
		t.Fatalf("submit job: %v", err)
	}

	claim, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "node-live"})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	time.Sleep(opts.LeaseTTL / 2)
	if err := sched.Heartbeat(ctx, scheduler.HeartbeatRequest{
		JobID:  claim.Job.ID,
		Ticket: claim.Job.Ticket,
		NodeID: "node-live",
	}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	time.Sleep(opts.LeaseTTL)

	if _, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "node-other"}); err == nil {
		t.Fatalf("job should still be running after heartbeat; expected no claimable jobs")
	}
}

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

// --- helpers ---

func newScheduler(t *testing.T, client *clientv3.Client) *scheduler.Scheduler {
	t.Helper()
	return mustNewScheduler(t, client, defaultOptions())
}

func mustNewScheduler(t *testing.T, client *clientv3.Client, opts scheduler.Options) *scheduler.Scheduler {
	t.Helper()
	sched, err := scheduler.New(client, opts)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	return sched
}

func defaultOptions() scheduler.Options {
	return scheduler.Options{
		JobsPrefix:      "mods/",
		QueuePrefix:     "queue/mods/",
		LeasesPrefix:    "leases/jobs/",
		NodesPrefix:     "nodes/",
		GCPrefix:        "gc/jobs/",
		LeaseTTL:        3 * time.Second,
		ClockSkewBuffer: 500 * time.Millisecond,
	}
}

func newTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir
	clientURL := mustParseURL("http://127.0.0.1:0")
	peerURL := mustParseURL("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "default"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "scheduler-test"
	cfg.LogLevel = "panic"
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{filepath.Join(dir, "etcd.log")}

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start etcd: %v", err)
	}
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Server.Stop()
		t.Fatalf("etcd start timeout")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		t.Fatalf("client: %v", err)
	}

	return e, client
}

func mustParseURL(raw string) url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *parsed
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
