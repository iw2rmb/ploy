package mods

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/internal/mods/plan"
)

func TestServiceSubmitPersistsTicketAndEnqueuesRootStages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	scheduler := newFakeScheduler()
	service := newTestService(t, client, scheduler)
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	}()

	spec := TicketSpec{
		TicketID:   "ticket-submit",
		Submitter:  "alice@example.com",
		Repository: "github.com/example/repo",
		Stages: []StageDefinition{
			{ID: plan.StageNamePlan, MaxAttempts: 1},
			{ID: plan.StageNameORWApply, Dependencies: []string{plan.StageNamePlan}, MaxAttempts: 1},
		},
	}

	status, err := service.Submit(ctx, spec)
	if err != nil {
		t.Fatalf("submit ticket: %v", err)
	}
	if status.TicketID != spec.TicketID {
		t.Fatalf("unexpected ticket id: got %s want %s", status.TicketID, spec.TicketID)
	}
	if status.State != TicketStatePending {
		t.Fatalf("expected pending ticket state, got %s", status.State)
	}
	if len(status.Stages) != 2 {
		t.Fatalf("expected 2 stages recorded, got %d", len(status.Stages))
	}
	stage := status.Stages[plan.StageNamePlan]
	if stage.State != StageStateQueued {
		t.Fatalf("expected root stage queued, got %s", stage.State)
	}

	jobs := scheduler.SubmittedJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 root job submitted, got %d", len(jobs))
	}
	if jobs[0].TicketID != spec.TicketID {
		t.Fatalf("unexpected job ticket: %s", jobs[0].TicketID)
	}
	if jobs[0].StageID != plan.StageNamePlan {
		t.Fatalf("unexpected job stage: %s", jobs[0].StageID)
	}
}

func TestServiceClaimStageUsesOptimisticConcurrency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	service := newTestService(t, client, newFakeScheduler())
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	}()

	spec := TicketSpec{
		TicketID:   "ticket-claim",
		Submitter:  "bob@example.com",
		Repository: "github.com/example/repo",
		Stages: []StageDefinition{
			{ID: plan.StageNamePlan, MaxAttempts: 2},
		},
	}

	if _, err := service.Submit(ctx, spec); err != nil {
		t.Fatalf("submit ticket: %v", err)
	}

	var success atomic.Int64
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			request := ClaimStageRequest{
				TicketID: spec.TicketID,
				StageID:  plan.StageNamePlan,
				JobID:    uuid.NewString(),
				NodeID:   "node-claim",
			}
			if _, err := service.ClaimStage(ctx, request); err == nil {
				success.Add(1)
			} else if !errors.Is(err, ErrStageAlreadyClaimed) {
				t.Errorf("unexpected claim error: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if success.Load() != 1 {
		t.Fatalf("expected exactly one claim success, got %d", success.Load())
	}
}

func TestServiceProcessJobCompletionSuccessTransitionsStageAndEnqueuesDependents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	scheduler := newFakeScheduler()
	service := newTestService(t, client, scheduler)
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	}()

	spec := TicketSpec{
		TicketID:   "ticket-complete-success",
		Submitter:  "carol@example.com",
		Repository: "github.com/example/repo",
		Stages: []StageDefinition{
			{ID: plan.StageNamePlan, MaxAttempts: 1},
			{ID: plan.StageNameORWApply, Dependencies: []string{plan.StageNamePlan}, MaxAttempts: 1},
			{ID: plan.StageNameLLMExec, Dependencies: []string{plan.StageNameORWApply}, MaxAttempts: 1},
		},
	}
	if _, err := service.Submit(ctx, spec); err != nil {
		t.Fatalf("submit ticket: %v", err)
	}

	jobID := uuid.NewString()
	if _, err := service.ClaimStage(ctx, ClaimStageRequest{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    jobID,
		NodeID:   "node-1",
	}); err != nil {
		t.Fatalf("claim stage: %v", err)
	}

	if err := service.ProcessJobCompletion(ctx, JobCompletion{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    jobID,
		State:    JobCompletionSucceeded,
	}); err != nil {
		t.Fatalf("process completion: %v", err)
	}

	stageStatus, err := service.StageStatus(ctx, spec.TicketID, plan.StageNamePlan)
	if err != nil {
		t.Fatalf("get stage status: %v", err)
	}
	if stageStatus.State != StageStateSucceeded {
		t.Fatalf("expected stage succeeded, got %s", stageStatus.State)
	}
	if stageStatus.CurrentJobID != "" {
		t.Fatalf("expected current job cleared, got %s", stageStatus.CurrentJobID)
	}

	jobs := scheduler.SubmittedJobs()
	if len(jobs) != 2 {
		t.Fatalf("expected dependent stage enqueued, got %d jobs", len(jobs))
	}
	if jobs[1].StageID != plan.StageNameORWApply {
		t.Fatalf("expected dependent orw-apply queued, got %s", jobs[1].StageID)
	}
}

func TestServiceProcessJobCompletionFailureRetries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	scheduler := newFakeScheduler()
	service := newTestService(t, client, scheduler)
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	}()

	spec := TicketSpec{
		TicketID:   "ticket-complete-failure",
		Submitter:  "dave@example.com",
		Repository: "github.com/example/repo",
		Stages: []StageDefinition{
			{ID: plan.StageNamePlan, MaxAttempts: 2},
		},
	}
	if _, err := service.Submit(ctx, spec); err != nil {
		t.Fatalf("submit ticket: %v", err)
	}

	jobID := uuid.NewString()
	if _, err := service.ClaimStage(ctx, ClaimStageRequest{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    jobID,
		NodeID:   "node-2",
	}); err != nil {
		t.Fatalf("claim stage: %v", err)
	}

	if err := service.ProcessJobCompletion(ctx, JobCompletion{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    jobID,
		State:    JobCompletionFailed,
		Error:    "step failed",
	}); err != nil {
		t.Fatalf("process completion: %v", err)
	}

	stageStatus, err := service.StageStatus(ctx, spec.TicketID, plan.StageNamePlan)
	if err != nil {
		t.Fatalf("stage status: %v", err)
	}
	if stageStatus.State != StageStateQueued {
		t.Fatalf("expected stage re-queued, got %s", stageStatus.State)
	}
	if stageStatus.Attempts != 1 {
		t.Fatalf("expected attempts incremented to 1, got %d", stageStatus.Attempts)
	}

	if len(scheduler.SubmittedJobs()) != 2 {
		t.Fatalf("expected retry job submitted, got %d", len(scheduler.SubmittedJobs()))
	}

	// Fail again to exhaust attempts.
	retryJobID := scheduler.SubmittedJobs()[1].JobID
	if _, err := service.ClaimStage(ctx, ClaimStageRequest{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    retryJobID,
		NodeID:   "node-2",
	}); err != nil {
		t.Fatalf("claim retry stage: %v", err)
	}
	if err := service.ProcessJobCompletion(ctx, JobCompletion{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    retryJobID,
		State:    JobCompletionFailed,
		Error:    "second failure",
	}); err != nil {
		t.Fatalf("process completion retry: %v", err)
	}

	ticketStatus, err := service.TicketStatus(ctx, spec.TicketID)
	if err != nil {
		t.Fatalf("ticket status: %v", err)
	}
	if ticketStatus.State != TicketStateFailed {
		t.Fatalf("expected ticket failed after retries, got %s", ticketStatus.State)
	}
}

func TestServiceCancelAndResume(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	scheduler := newFakeScheduler()
	service := newTestService(t, client, scheduler)
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	}()

	spec := TicketSpec{
		TicketID:   "ticket-cancel",
		Submitter:  "eve@example.com",
		Repository: "github.com/example/repo",
		Stages: []StageDefinition{
			{ID: plan.StageNamePlan, MaxAttempts: 1},
			{ID: plan.StageNameHuman, Dependencies: []string{plan.StageNamePlan}, MaxAttempts: 1},
		},
	}
	if _, err := service.Submit(ctx, spec); err != nil {
		t.Fatalf("submit ticket: %v", err)
	}

	if err := service.Cancel(ctx, spec.TicketID); err != nil {
		t.Fatalf("cancel ticket: %v", err)
	}
	status, err := service.TicketStatus(ctx, spec.TicketID)
	if err != nil {
		t.Fatalf("ticket status: %v", err)
	}
	if status.State != TicketStateCancelled && status.State != TicketStateCancelling {
		t.Fatalf("expected cancelling or cancelled, got %s", status.State)
	}

	// resume should requeue pending stages
	if _, err := service.Resume(ctx, spec.TicketID); err != nil {
		t.Fatalf("resume ticket: %v", err)
	}
	resumed, err := service.TicketStatus(ctx, spec.TicketID)
	if err != nil {
		t.Fatalf("ticket status after resume: %v", err)
	}
	if resumed.State != TicketStatePending {
		t.Fatalf("expected ticket pending after resume, got %s", resumed.State)
	}
	if len(scheduler.SubmittedJobs()) < 2 {
		t.Fatalf("expected resumed stage enqueued, got %d jobs", len(scheduler.SubmittedJobs()))
	}
}

// --- helpers ---

func newTestService(t *testing.T, client *clientv3.Client, scheduler *fakeScheduler) *Service {
	t.Helper()
	service, err := NewService(client, Options{
		Prefix:    "mods/",
		Scheduler: scheduler,
		Clock:     func() time.Time { return time.Unix(0, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

func newTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir
	clientURL := mustURL(t, "http://127.0.0.1:0")
	peerURL := mustURL(t, "http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "mods-control-plane"
	cfg.InitialCluster = cfg.Name + "=" + peerURL.String()
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{filepath.Join(dir, "etcd.log")}
	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start etcd: %v", err)
	}
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for etcd ready")
	}
	endpoint := e.Clients[0].Addr().String()
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new etcd client: %v", err)
	}
	return e, client
}

func mustURL(t *testing.T, raw string) url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return *parsed
}

type fakeScheduler struct {
	mu   sync.Mutex
	jobs []StageJobSpec
}

func newFakeScheduler() *fakeScheduler {
	return &fakeScheduler{jobs: make([]StageJobSpec, 0)}
}

func (s *fakeScheduler) SubmitStageJob(ctx context.Context, spec StageJobSpec) (StageJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := StageJob{
		JobID:    spec.JobID,
		TicketID: spec.TicketID,
		StageID:  spec.StageID,
	}
	if job.JobID == "" {
		job.JobID = uuid.NewString()
	}
	spec.JobID = job.JobID
	s.jobs = append(s.jobs, spec)
	return job, nil
}

func (s *fakeScheduler) SubmittedJobs() []StageJobSpec {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StageJobSpec, len(s.jobs))
	copy(out, s.jobs)
	return out
}
