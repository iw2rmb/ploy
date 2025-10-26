package scheduler_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/metrics"
)

func TestSchedulerMetricsQueueAndRetries(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	reg := prometheus.NewRegistry()
	recorder, err := metrics.NewSchedulerMetrics(reg)
	if err != nil {
		t.Fatalf("new scheduler metrics: %v", err)
	}

	now := time.Date(2025, 10, 22, 17, 0, 0, 0, time.UTC)
	opts := defaultOptions()
	opts.LeaseTTL = time.Second
	opts.ClockSkewBuffer = 0
	opts.Now = func() time.Time { return now }
	opts.Metrics = recorder

	sched := mustNewScheduler(t, client, opts)
	defer func() { _ = sched.Close() }()

	if _, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-metrics",
		StepID:      "plan",
		Priority:    "default",
		MaxAttempts: 2,
	}); err != nil {
		t.Fatalf("submit job: %v", err)
	}

	if value, ok := gaugeValue(reg, "ploy_controlplane_queue_depth", map[string]string{"priority": "default"}); !ok || math.Abs(value-1.0) > 1e-6 {
		t.Fatalf("expected queue depth gauge 1 after submit, got %.4f (ok=%v)", value, ok)
	}

	now = now.Add(4 * time.Second)

	if _, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "node-metrics"}); err != nil {
		t.Fatalf("claim job: %v", err)
	}

	if value, ok := gaugeValue(reg, "ploy_controlplane_queue_depth", map[string]string{"priority": "default"}); !ok || math.Abs(value) > 1e-6 {
		t.Fatalf("expected queue depth gauge 0 after claim, got %.4f (ok=%v)", value, ok)
	}

	if count, sum, ok := histogramValue(reg, "ploy_controlplane_claim_latency_seconds", map[string]string{"priority": "default"}); !ok {
		t.Fatalf("expected claim latency histogram sample")
	} else if count != 1 {
		t.Fatalf("expected claim latency count 1, got %d", count)
	} else if diff := math.Abs(sum - 4.0); diff > 0.25 {
		t.Fatalf("expected claim latency sum ~4.0, got %.4f (diff %.4f)", sum, diff)
	}

	time.Sleep(opts.LeaseTTL + 200*time.Millisecond)

	waitForCondition(t, 5*time.Second, func() bool {
		if _, ok := gaugeValue(reg, "ploy_controlplane_queue_depth", map[string]string{"priority": "default"}); !ok {
			return false
		}
		value, _ := gaugeValue(reg, "ploy_controlplane_queue_depth", map[string]string{"priority": "default"})
		return value >= 1.0
	})

	waitForCondition(t, 5*time.Second, func() bool {
		count, _, ok := histogramValue(reg, "ploy_controlplane_claim_latency_seconds", map[string]string{"priority": "default"})
		if !ok {
			return false
		}
		value, ok := counterValue(reg, "ploy_controlplane_job_retry_total", map[string]string{"priority": "default", "reason": "lease_expired"})
		return ok && value >= 1 && count >= 1
	})
}

func TestSchedulerRecordsShiftMetrics(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	reg := prometheus.NewRegistry()
	recorder, err := metrics.NewSchedulerMetrics(reg)
	if err != nil {
		t.Fatalf("new scheduler metrics: %v", err)
	}

	now := time.Date(2025, 10, 22, 18, 0, 0, 0, time.UTC)
	opts := defaultOptions()
	opts.Now = func() time.Time { return now }
	opts.Metrics = recorder

	sched := mustNewScheduler(t, client, opts)
	defer func() { _ = sched.Close() }()

	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-shift",
		StepID:      "test",
		Priority:    "batch",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	claim, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "node-shift"})
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}

	shiftDuration := 2*time.Second + 500*time.Millisecond

	result, err := sched.CompleteJob(ctx, scheduler.CompleteRequest{
		JobID:  claim.Job.ID,
		Ticket: job.Ticket,
		NodeID: "node-shift",
		State:  scheduler.JobStateSucceeded,
		Shift: &scheduler.ShiftMetrics{
			Result:   scheduler.ShiftResultPassed,
			Duration: shiftDuration,
		},
	})
	if err != nil {
		t.Fatalf("complete job: %v", err)
	}

	if result.Shift == nil {
		t.Fatalf("expected shift summary on completion result")
	}
	if result.Shift.Result != scheduler.ShiftResultPassed {
		t.Fatalf("unexpected result.Shift.Result: %s", result.Shift.Result)
	}
	if diff := math.Abs(result.Shift.Duration.Seconds() - shiftDuration.Seconds()); diff > 1e-6 {
		t.Fatalf("unexpected shift duration %.6f", result.Shift.Duration.Seconds())
	}

	stored, err := sched.GetJob(ctx, job.Ticket, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if stored.Shift == nil {
		t.Fatalf("expected stored job to include shift summary")
	}
	if stored.Shift.Result != scheduler.ShiftResultPassed {
		t.Fatalf("unexpected stored.Shift.Result: %s", stored.Shift.Result)
	}
	if diff := math.Abs(stored.Shift.Duration.Seconds() - shiftDuration.Seconds()); diff > 1e-6 {
		t.Fatalf("unexpected stored shift duration %.6f", stored.Shift.Duration.Seconds())
	}

	count, sum, ok := histogramValue(reg, "ploy_controlplane_shift_duration_seconds", map[string]string{"step_id": job.StepID, "result": scheduler.ShiftResultPassed})
	if !ok {
		t.Fatalf("expected shift duration histogram sample")
	}
	if count != 1 {
		t.Fatalf("expected shift histogram count 1, got %d", count)
	}
	if diff := math.Abs(sum - shiftDuration.Seconds()); diff > 0.05 {
		t.Fatalf("expected shift histogram sum %.3f, got %.3f (diff %.3f)", shiftDuration.Seconds(), sum, diff)
	}
}
