package scheduler_test

import (
	"context"
	"sync"
	"testing"
	"time"

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
