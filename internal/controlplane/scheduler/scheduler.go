package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path"
	"strings"
	"sync"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/google/uuid"

	metricsx "github.com/iw2rmb/ploy/internal/metrics"
)

const (
	defaultRetentionWindow     = 24 * time.Hour
	jobRetryReasonLeaseExpired = "lease_expired"
)

// Scheduler coordinates job submission, claims, and lifecycle management backed by etcd.
type Scheduler struct {
	client *clientv3.Client

	jobsPrefix   string
	queuePrefix  string
	leasesPrefix string
	nodesPrefix  string
	gcPrefix     string

	leaseTTL    time.Duration
	clockSkew   time.Duration
	now         func() time.Time
	idGenerator func() string

	ctx     context.Context
	cancel  context.CancelFunc
	metrics metricsx.SchedulerRecorder
	wg      sync.WaitGroup
}

// New constructs a scheduler with the provided etcd client and options.
func New(client *clientv3.Client, opts Options) (*Scheduler, error) {
	if client == nil {
		return nil, errors.New("scheduler: etcd client is required")
	}

	cfg := compileOptions(opts)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		client:       client,
		jobsPrefix:   cfg.jobsPrefix,
		queuePrefix:  cfg.queuePrefix,
		leasesPrefix: cfg.leasesPrefix,
		nodesPrefix:  cfg.nodesPrefix,
		gcPrefix:     cfg.gcPrefix,
		leaseTTL:     cfg.leaseTTL,
		clockSkew:    cfg.clockSkew,
		now:          cfg.now,
		idGenerator:  cfg.idGenerator,
		ctx:          ctx,
		cancel:       cancel,
		metrics:      cfg.metrics,
	}

	s.wg.Add(1)
	go s.watchLeases()

	s.wg.Add(1)
	go s.watchGCMarkers()

	s.wg.Add(1)
	go s.watchNodeStatus()

	return s, nil
}

// Close stops background watchers.
func (s *Scheduler) Close() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

// SubmitJob enqueues a new job and returns its persisted record.
func (s *Scheduler) SubmitJob(ctx context.Context, spec JobSpec) (*Job, error) {
	if strings.TrimSpace(spec.Ticket) == "" {
		return nil, errors.New("scheduler: ticket is required")
	}
	if strings.TrimSpace(spec.StepID) == "" {
		return nil, errors.New("scheduler: step id is required")
	}

	priority := strings.TrimSpace(spec.Priority)
	if priority == "" {
		priority = "default"
	}

	maxAttempts := spec.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	jobID := s.idGenerator()
	now := s.now().UTC()

	record := jobRecord{
		ID:             jobID,
		Ticket:         spec.Ticket,
		StepID:         spec.StepID,
		Priority:       priority,
		State:          JobStateQueued,
		CreatedAt:      encodeTime(now),
		EnqueuedAt:     encodeTime(now),
		RetryAttempt:   0,
		MaxAttempts:    maxAttempts,
		Metadata:       cloneMap(spec.Metadata),
		Artifacts:      map[string]string{},
		Bundles:        map[string]bundleRecord{},
		ExpiresAt:      "",
		NodeSnapshot:   nil,
		LeaseExpiresAt: "",
	}

	queue := queueEntry{
		JobID:        jobID,
		Ticket:       spec.Ticket,
		StepID:       spec.StepID,
		Priority:     priority,
		RetryAttempt: 0,
		MaxAttempts:  maxAttempts,
		EnqueuedAt:   encodeTime(now),
		Metadata:     cloneMap(spec.Metadata),
	}

	jobKey := s.jobKey(spec.Ticket, jobID)
	queueKey := s.queueKey(priority, jobID)

	jobBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal job record: %w", err)
	}
	queueBytes, err := json.Marshal(queue)
	if err != nil {
		return nil, fmt.Errorf("marshal queue entry: %w", err)
	}

	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.CreateRevision(jobKey), "=", 0),
		clientv3.Compare(clientv3.CreateRevision(queueKey), "=", 0),
	).Then(
		clientv3.OpPut(jobKey, string(jobBytes)),
		clientv3.OpPut(queueKey, string(queueBytes)),
	)

	resp, err := txn.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit job submit txn: %w", err)
	}
	if !resp.Succeeded {
		return nil, fmt.Errorf("scheduler: job already exists for ticket %s step %s", spec.Ticket, spec.StepID)
	}

	s.metrics.QueueEnqueued(priority)

	return record.toJob(), nil
}

// ClaimNext attempts to claim the next available job for the provided node.
func (s *Scheduler) ClaimNext(ctx context.Context, req ClaimRequest) (*ClaimResult, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		return nil, errors.New("scheduler: node id is required")
	}

	for attempts := 0; attempts < 8; attempts++ {
		job, err := s.tryClaimOnce(ctx, req)
		if err == ErrNoJobs {
			return nil, err
		}
		if err != nil {
			if errors.Is(err, errRetryClaim) {
				continue
			}
			return nil, err
		}
		return job, nil
	}
	return nil, ErrNoJobs
}

// Heartbeat renews the lease for a running job.
func (s *Scheduler) Heartbeat(ctx context.Context, req HeartbeatRequest) error {
	if strings.TrimSpace(req.JobID) == "" {
		return errors.New("scheduler: heartbeat requires job id")
	}
	if strings.TrimSpace(req.NodeID) == "" {
		return errors.New("scheduler: heartbeat requires node id")
	}

	var jobKey string
	if trimmed := strings.TrimSpace(req.Ticket); trimmed != "" {
		jobKey = s.jobKey(trimmed, req.JobID)
	} else {
		var err error
		jobKey, _, err = s.lookupJobKey(ctx, req.JobID)
		if err != nil {
			return err
		}
	}

	resp, err := s.client.Get(ctx, jobKey)
	if err != nil {
		return fmt.Errorf("scheduler: fetch job heartbeat: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return fmt.Errorf("scheduler: job %s not found", req.JobID)
	}

	record, err := decodeJobRecord(resp.Kvs[0].Value)
	if err != nil {
		return fmt.Errorf("scheduler: decode job heartbeat: %w", err)
	}
	if record.State != JobStateRunning {
		return fmt.Errorf("scheduler: job %s not running", req.JobID)
	}
	if record.ClaimedBy != req.NodeID {
		return fmt.Errorf("scheduler: job %s owned by %s", req.JobID, record.ClaimedBy)
	}

	leaseID := clientv3.LeaseID(record.LeaseID)
	if leaseID == clientv3.NoLease {
		return fmt.Errorf("scheduler: job %s missing lease", req.JobID)
	}

	if _, err := s.client.KeepAliveOnce(ctx, leaseID); err != nil {
		return fmt.Errorf("scheduler: heartbeat lease: %w", err)
	}
	ttl, err := s.client.TimeToLive(ctx, leaseID)
	if err != nil {
		return fmt.Errorf("scheduler: ttl lookup: %w", err)
	}

	now := s.now().UTC()
	record.LeaseExpiresAt = encodeTime(now.Add(time.Duration(ttl.TTL) * time.Second))

	if snapshot := s.captureNodeSnapshot(ctx, req.NodeID); snapshot != nil {
		record.NodeSnapshot = snapshot
	}

	jobBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("scheduler: marshal heartbeat job: %w", err)
	}

	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(jobKey), "=", resp.Kvs[0].ModRevision),
	).Then(
		clientv3.OpPut(jobKey, string(jobBytes)),
	)

	txnResp, err := txn.Commit()
	if err != nil {
		return fmt.Errorf("scheduler: heartbeat commit: %w", err)
	}
	if !txnResp.Succeeded {
		return fmt.Errorf("scheduler: heartbeat conflict for job %s", req.JobID)
	}
	return nil
}

// CompleteJob finalises a job with the provided terminal state.
func (s *Scheduler) CompleteJob(ctx context.Context, req CompleteRequest) (*Job, error) {
	if strings.TrimSpace(req.JobID) == "" {
		return nil, errors.New("scheduler: job id required")
	}
	if strings.TrimSpace(req.NodeID) == "" {
		return nil, errors.New("scheduler: node id required")
	}
	if req.State != JobStateSucceeded && req.State != JobStateFailed && req.State != JobStateInspectionReady {
		return nil, fmt.Errorf("scheduler: invalid completion state %s", req.State)
	}

	var (
		jobKey string
		err    error
	)
	if trimmed := strings.TrimSpace(req.Ticket); trimmed != "" {
		jobKey = s.jobKey(trimmed, req.JobID)
	} else {
		jobKey, _, err = s.lookupJobKey(ctx, req.JobID)
		if err != nil {
			return nil, err
		}
	}

	resp, err := s.client.Get(ctx, jobKey)
	if err != nil {
		return nil, fmt.Errorf("scheduler: fetch job complete: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("scheduler: job %s not found", req.JobID)
	}

	record, err := decodeJobRecord(resp.Kvs[0].Value)
	if err != nil {
		return nil, fmt.Errorf("scheduler: decode job complete: %w", err)
	}
	if record.State != JobStateRunning {
		return nil, fmt.Errorf("scheduler: job %s not running", req.JobID)
	}
	if record.ClaimedBy != req.NodeID {
		return nil, fmt.Errorf("scheduler: job %s owned by %s", req.JobID, record.ClaimedBy)
	}

	completedAt := s.now().UTC()
	record.State = req.State
	record.CompletedAt = encodeTime(completedAt)
	record.LeaseID = 0
	record.LeaseExpiresAt = ""
	if req.Artifacts != nil {
		record.Artifacts = cloneMap(req.Artifacts)
	}
	if req.Error != nil {
		record.Error = req.Error
	}
	if len(req.Bundles) > 0 {
		record.Bundles = normalizeBundleRecords(req.Bundles, completedAt)
	}
	if req.Inspection && req.State == JobStateFailed {
		record.State = JobStateInspectionReady
	}
	expiry := completedAt.Add(defaultRetentionWindow).UTC()
	if derived := computeRetentionExpiry(record.Bundles, completedAt); !derived.IsZero() {
		expiry = derived
	}
	if expiry.IsZero() {
		record.ExpiresAt = ""
	} else {
		record.ExpiresAt = encodeTime(expiry)
	}
	record.Retention = deriveRetentionRecord(record.Bundles, expiry, record.State == JobStateInspectionReady)

	shiftDuration := time.Duration(0)
	shiftResult := ""
	if req.Shift != nil {
		shiftDuration = req.Shift.Duration
		if shiftDuration < 0 {
			shiftDuration = 0
		}
		shiftResult = normalizeShiftResult(req.Shift.Result, record.State)
		if shiftDuration > 0 || shiftResult != "" {
			record.Shift = &shiftRecord{
				Result:          shiftResult,
				DurationSeconds: shiftDuration.Seconds(),
			}
		}
	}

	if snapshot := s.captureNodeSnapshot(ctx, req.NodeID); snapshot != nil {
		record.NodeSnapshot = snapshot
	}

	jobBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("scheduler: marshal completion job: %w", err)
	}

	// GC marker
	gcPayload := gcEntry{
		JobID:      record.ID,
		Ticket:     record.Ticket,
		State:      record.State,
		ExpiresAt:  encodeTime(expiry),
		FinalState: string(record.State),
	}
	gcBytes, err := json.Marshal(gcPayload)
	if err != nil {
		return nil, fmt.Errorf("scheduler: marshal gc payload: %w", err)
	}

	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(jobKey), "=", resp.Kvs[0].ModRevision),
	).Then(
		clientv3.OpPut(jobKey, string(jobBytes)),
		clientv3.OpPut(s.gcKey(record.ID), string(gcBytes)),
	)

	txnResp, err := txn.Commit()
	if err != nil {
		return nil, fmt.Errorf("scheduler: complete commit: %w", err)
	}
	if !txnResp.Succeeded {
		return nil, fmt.Errorf("scheduler: completion conflict for job %s", req.JobID)
	}
	if shiftDuration > 0 {
		s.metrics.ObserveShiftDuration(record.StepID, shiftResult, shiftDuration)
	}

	if record.LeaseID != 0 {
		if _, err := s.client.Revoke(ctx, clientv3.LeaseID(record.LeaseID)); err != nil {
			log.Printf("scheduler: revoke lease %d: %v", record.LeaseID, err)
		}
	}

	return record.toJob(), nil
}

// GetJob returns the current job record.
func (s *Scheduler) GetJob(ctx context.Context, ticket, jobID string) (*Job, error) {
	jobKey := s.jobKey(ticket, jobID)
	resp, err := s.client.Get(ctx, jobKey)
	if err != nil {
		return nil, fmt.Errorf("scheduler: get job: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("scheduler: job %s not found", jobID)
	}
	record, err := decodeJobRecord(resp.Kvs[0].Value)
	if err != nil {
		return nil, fmt.Errorf("scheduler: decode job: %w", err)
	}
	return record.toJob(), nil
}

// ListJobs returns all jobs for a ticket ordered by etcd key order.
func (s *Scheduler) ListJobs(ctx context.Context, ticket string) ([]*Job, error) {
	if strings.TrimSpace(ticket) == "" {
		return nil, errors.New("scheduler: ticket is required")
	}
	prefix := path.Join(s.jobsPrefix, ticket, "jobs")
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("scheduler: list jobs: %w", err)
	}
	jobs := make([]*Job, 0, resp.Count)
	for _, kv := range resp.Kvs {
		record, err := decodeJobRecord(kv.Value)
		if err != nil {
			return nil, fmt.Errorf("scheduler: decode job listing: %w", err)
		}
		jobs = append(jobs, record.toJob())
	}
	return jobs, nil
}

// RunningJobsForNode returns all jobs currently running on the provided node.
func (s *Scheduler) RunningJobsForNode(ctx context.Context, nodeID string) ([]*Job, error) {
	if strings.TrimSpace(nodeID) == "" {
		return nil, errors.New("scheduler: node id is required")
	}
	prefix := s.jobsPrefix + "/"
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("scheduler: list jobs for node: %w", err)
	}
	jobs := make([]*Job, 0, resp.Count)
	for _, kv := range resp.Kvs {
		record, err := decodeJobRecord(kv.Value)
		if err != nil {
			return nil, fmt.Errorf("scheduler: decode job for node listing: %w", err)
		}
		if record.State != JobStateRunning {
			continue
		}
		if record.ClaimedBy != nodeID {
			continue
		}
		jobs = append(jobs, record.toJob())
	}
	return jobs, nil
}

// watchLeases monitors lease prefix deletions to requeue expired jobs.
func (s *Scheduler) watchLeases() {
	defer s.wg.Done()

	watch := s.client.Watch(s.ctx, s.leasesPrefix, clientv3.WithPrefix(), clientv3.WithPrevKV())
	for {
		select {
		case <-s.ctx.Done():
			return
		case resp, ok := <-watch:
			if !ok || resp.Canceled {
				return
			}
			if resp.Err() != nil {
				log.Printf("scheduler: lease watch error: %v", resp.Err())
				continue
			}
			for _, ev := range resp.Events {
				if ev.Type != mvccpb.DELETE || ev.PrevKv == nil {
					continue
				}
				var lease leaseEntry
				if err := json.Unmarshal(ev.PrevKv.Value, &lease); err != nil {
					log.Printf("scheduler: decode lease entry: %v", err)
					continue
				}
				if lease.JobID == "" || lease.Ticket == "" {
					continue
				}
				ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				if err := s.handleLeaseExpiry(ctx, lease, clientv3.LeaseID(ev.PrevKv.Lease)); err != nil {
					log.Printf("scheduler: handle lease expiry: %v", err)
				}
				cancel()
			}
		}
	}
}

func (s *Scheduler) handleLeaseExpiry(ctx context.Context, lease leaseEntry, leaseID clientv3.LeaseID) error {
	jobKey := s.jobKey(lease.Ticket, lease.JobID)
	resp, err := s.client.Get(ctx, jobKey)
	if err != nil {
		return fmt.Errorf("fetch job for lease expiry: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil
	}
	record, err := decodeJobRecord(resp.Kvs[0].Value)
	if err != nil {
		return fmt.Errorf("decode job for lease expiry: %w", err)
	}
	if record.State != JobStateRunning {
		return nil
	}
	if clientv3.LeaseID(record.LeaseID) != leaseID {
		return nil
	}

	now := s.now().UTC()
	record.ClaimedBy = ""
	record.ClaimedAt = ""
	record.LeaseID = 0
	record.LeaseExpiresAt = ""
	record.ExpiresAt = ""
	record.NodeSnapshot = nil
	record.RetryAttempt++
	record.Shift = nil

	var (
		queuePut clientv3.Op
		errMsg   *JobError
		newState JobState
		requeued bool
	)

	if record.RetryAttempt >= record.MaxAttempts {
		newState = JobStateFailed
		errMsg = &JobError{
			Reason:  "lease_expired",
			Message: "worker lease expired without heartbeat",
		}
		record.CompletedAt = encodeTime(now)
	} else {
		newState = JobStateQueued
		queue := queueEntry{
			JobID:        record.ID,
			Ticket:       record.Ticket,
			StepID:       record.StepID,
			Priority:     lease.Priority,
			RetryAttempt: record.RetryAttempt,
			MaxAttempts:  record.MaxAttempts,
			EnqueuedAt:   encodeTime(now),
			Metadata:     cloneMap(record.Metadata),
		}
		queueBytes, err := json.Marshal(queue)
		if err != nil {
			return fmt.Errorf("marshal queue requeue: %w", err)
		}
		queuePut = clientv3.OpPut(s.queueKey(lease.Priority, record.ID), string(queueBytes))
		record.EnqueuedAt = queue.EnqueuedAt
		requeued = true
	}

	record.State = newState
	if errMsg != nil {
		record.Error = errMsg
	}

	jobBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal job requeue: %w", err)
	}

	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(jobKey), "=", resp.Kvs[0].ModRevision),
	)

	ops := []clientv3.Op{clientv3.OpPut(jobKey, string(jobBytes))}
	if queuePut.KeyBytes() != nil {
		ops = append(ops, queuePut)
	}

	txn = txn.Then(ops...)

	respTxn, err := txn.Commit()
	if err != nil {
		return err
	}
	_ = respTxn
	if requeued {
		s.metrics.QueueEnqueued(lease.Priority)
	}
	s.metrics.ObserveJobRetry(lease.Priority, jobRetryReasonLeaseExpired)
	return nil
}

// watchGCMarkers keeps job expiry metadata aligned with GC markers.
func (s *Scheduler) watchGCMarkers() {
	defer s.wg.Done()

	watch := s.client.Watch(s.ctx, s.gcPrefix, clientv3.WithPrefix())
	for {
		select {
		case <-s.ctx.Done():
			return
		case resp, ok := <-watch:
			if !ok || resp.Canceled {
				return
			}
			if err := resp.Err(); err != nil {
				log.Printf("scheduler: gc watch error: %v", err)
				continue
			}
			for _, ev := range resp.Events {
				if ev.Type != mvccpb.PUT || ev.Kv == nil {
					continue
				}
				var marker gcEntry
				if err := json.Unmarshal(ev.Kv.Value, &marker); err != nil {
					log.Printf("scheduler: decode gc entry: %v", err)
					continue
				}
				if strings.TrimSpace(marker.JobID) == "" || strings.TrimSpace(marker.Ticket) == "" {
					continue
				}
				ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				if err := s.syncJobExpiry(ctx, marker); err != nil {
					log.Printf("scheduler: sync job expiry: %v", err)
				}
				cancel()
			}
		}
	}
}

func (s *Scheduler) syncJobExpiry(ctx context.Context, marker gcEntry) error {
	ticket := strings.TrimSpace(marker.Ticket)
	jobID := strings.TrimSpace(marker.JobID)
	if ticket == "" || jobID == "" {
		return nil
	}
	jobKey := s.jobKey(ticket, jobID)
	resp, err := s.client.Get(ctx, jobKey)
	if err != nil {
		return fmt.Errorf("fetch job for gc sync: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil
	}
	record, err := decodeJobRecord(resp.Kvs[0].Value)
	if err != nil {
		return fmt.Errorf("decode job for gc sync: %w", err)
	}

	expiry := strings.TrimSpace(marker.ExpiresAt)
	changed := false
	if expiry != "" {
		if record.ExpiresAt != expiry {
			record.ExpiresAt = expiry
			changed = true
		}
		if record.Retention != nil && record.Retention.ExpiresAt != expiry {
			record.Retention.ExpiresAt = expiry
			changed = true
		}
	} else {
		if record.ExpiresAt != "" {
			record.ExpiresAt = ""
			changed = true
		}
		if record.Retention != nil && record.Retention.ExpiresAt != "" {
			record.Retention.ExpiresAt = ""
			changed = true
		}
	}
	if !changed {
		return nil
	}

	jobBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal job for gc sync: %w", err)
	}
	_, err = s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(jobKey), "=", resp.Kvs[0].ModRevision),
	).Then(
		clientv3.OpPut(jobKey, string(jobBytes)),
	).Commit()
	return err
}

// watchNodeStatus mirrors node health snapshots into running jobs.
func (s *Scheduler) watchNodeStatus() {
	defer s.wg.Done()

	watch := s.client.Watch(s.ctx, s.nodesPrefix+"/", clientv3.WithPrefix())
	for {
		select {
		case <-s.ctx.Done():
			return
		case resp, ok := <-watch:
			if !ok || resp.Canceled {
				return
			}
			if err := resp.Err(); err != nil {
				log.Printf("scheduler: node status watch error: %v", err)
				continue
			}
			for _, ev := range resp.Events {
				if ev.Type != mvccpb.PUT || ev.Kv == nil {
					continue
				}
				key := string(ev.Kv.Key)
				if !strings.HasSuffix(key, "/status") {
					continue
				}
				trimmed := strings.TrimPrefix(key, s.nodesPrefix)
				trimmed = strings.TrimPrefix(trimmed, "/")
				parts := strings.Split(trimmed, "/")
				if len(parts) < 2 {
					continue
				}
				nodeID := strings.TrimSpace(parts[0])
				if nodeID == "" {
					continue
				}
				status := decodeKVMap(ev.Kv.Value)
				if len(status) == 0 {
					continue
				}
				observed := snapshotTimestamp(status, s.now())
				ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				if err := s.applyNodeStatus(ctx, nodeID, status, observed); err != nil {
					log.Printf("scheduler: apply node status: %v", err)
				}
				cancel()
			}
		}
	}
}

func (s *Scheduler) applyNodeStatus(ctx context.Context, nodeID string, status map[string]any, observed string) error {
	if len(status) == 0 {
		return nil
	}
	if strings.TrimSpace(observed) == "" {
		observed = encodeTime(s.now())
	}
	prefix := s.jobsPrefix + "/"
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("fetch jobs for node status: %w", err)
	}
	for _, kv := range resp.Kvs {
		record, err := decodeJobRecord(kv.Value)
		if err != nil {
			log.Printf("scheduler: decode job for node status: %v", err)
			continue
		}
		if record.State != JobStateRunning || record.ClaimedBy != nodeID {
			continue
		}
		if record.NodeSnapshot == nil {
			record.NodeSnapshot = &nodeSnapshotRecord{NodeID: nodeID}
		} else if strings.TrimSpace(record.NodeSnapshot.NodeID) == "" {
			record.NodeSnapshot.NodeID = nodeID
		}
		record.NodeSnapshot.Status = cloneAnyMap(status)
		record.NodeSnapshot.StatusAt = observed

		jobBytes, err := json.Marshal(record)
		if err != nil {
			log.Printf("scheduler: marshal job node status: %v", err)
			continue
		}
		jobKey := string(kv.Key)
		_, err = s.client.Txn(ctx).If(
			clientv3.Compare(clientv3.ModRevision(jobKey), "=", kv.ModRevision),
		).Then(
			clientv3.OpPut(jobKey, string(jobBytes)),
		).Commit()
		if err != nil {
			log.Printf("scheduler: update node status for job %s: %v", record.ID, err)
		}
	}
	return nil
}

// captureNodeSnapshot fetches node capacity and status to persist with a job mutation.
func (s *Scheduler) captureNodeSnapshot(ctx context.Context, nodeID string) *nodeSnapshotRecord {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil
	}

	baseCtx := ctx
	if baseCtx == nil {
		baseCtx = s.ctx
	}

	snapshot := &nodeSnapshotRecord{NodeID: nodeID}

	capCtx, cancelCap := context.WithTimeout(baseCtx, 2*time.Second)
	capResp, err := s.client.Get(capCtx, path.Join(s.nodesPrefix, nodeID, "capacity"))
	cancelCap()
	if err != nil {
		log.Printf("scheduler: fetch node capacity: %v", err)
	} else if len(capResp.Kvs) > 0 {
		snapshot.Capacity = decodeKVMap(capResp.Kvs[0].Value)
		if len(snapshot.Capacity) > 0 {
			snapshot.CapacityAt = snapshotTimestamp(snapshot.Capacity, s.now())
		}
	}

	statusCtx, cancelStatus := context.WithTimeout(baseCtx, 2*time.Second)
	statusResp, err := s.client.Get(statusCtx, path.Join(s.nodesPrefix, nodeID, "status"))
	cancelStatus()
	if err != nil {
		log.Printf("scheduler: fetch node status: %v", err)
	} else if len(statusResp.Kvs) > 0 {
		snapshot.Status = decodeKVMap(statusResp.Kvs[0].Value)
		if len(snapshot.Status) > 0 {
			snapshot.StatusAt = snapshotTimestamp(snapshot.Status, s.now())
		}
	}

	if len(snapshot.Capacity) == 0 && len(snapshot.Status) == 0 {
		return nil
	}
	if snapshot.CapacityAt == "" && len(snapshot.Capacity) > 0 {
		snapshot.CapacityAt = encodeTime(s.now())
	}
	if snapshot.StatusAt == "" && len(snapshot.Status) > 0 {
		snapshot.StatusAt = encodeTime(s.now())
	}
	return snapshot
}

func (s *Scheduler) tryClaimOnce(ctx context.Context, req ClaimRequest) (*ClaimResult, error) {
	resp, err := s.client.Get(ctx, s.queuePrefix, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByCreateRevision, clientv3.SortAscend), clientv3.WithLimit(1))
	if err != nil {
		return nil, fmt.Errorf("scheduler: query queue: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, ErrNoJobs
	}

	kv := resp.Kvs[0]
	var entry queueEntry
	if err := json.Unmarshal(kv.Value, &entry); err != nil {
		_, _ = s.client.Delete(ctx, string(kv.Key))
		return nil, errRetryClaim
	}

	jobKey := s.jobKey(entry.Ticket, entry.JobID)
	jobResp, err := s.client.Get(ctx, jobKey)
	if err != nil {
		return nil, fmt.Errorf("scheduler: fetch job %s: %w", entry.JobID, err)
	}
	if len(jobResp.Kvs) == 0 {
		_, _ = s.client.Delete(ctx, string(kv.Key))
		return nil, errRetryClaim
	}

	record, err := decodeJobRecord(jobResp.Kvs[0].Value)
	if err != nil {
		_, _ = s.client.Delete(ctx, string(kv.Key))
		return nil, errRetryClaim
	}
	if record.State != JobStateQueued {
		_, _ = s.client.Delete(ctx, string(kv.Key))
		return nil, errRetryClaim
	}

	ttlSecs := int64(s.leaseTTL.Seconds())
	if ttlSecs <= 0 {
		ttlSecs = 1
	}
	lease, err := s.client.Grant(ctx, ttlSecs)
	if err != nil {
		return nil, fmt.Errorf("scheduler: grant lease: %w", err)
	}

	enqueueAt := decodeTime(record.EnqueuedAt)
	now := s.now().UTC()
	latency := time.Duration(0)
	if !enqueueAt.IsZero() {
		latency = now.Sub(enqueueAt)
		if latency < 0 {
			latency = 0
		}
	}
	record.State = JobStateRunning
	record.ClaimedBy = req.NodeID
	record.ClaimedAt = encodeTime(now)
	record.LeaseID = int64(lease.ID)
	record.LeaseExpiresAt = encodeTime(now.Add(s.leaseTTL + s.clockSkew))
	record.ExpiresAt = ""
	if snapshot := s.captureNodeSnapshot(ctx, req.NodeID); snapshot != nil {
		record.NodeSnapshot = snapshot
	}

	jobBytes, err := json.Marshal(record)
	if err != nil {
		_, _ = s.client.Revoke(ctx, lease.ID)
		return nil, fmt.Errorf("scheduler: marshal job claim: %w", err)
	}

	leasePayload := leaseEntry{
		JobID:    record.ID,
		Ticket:   record.Ticket,
		Priority: entry.Priority,
	}
	leaseBytes, err := json.Marshal(leasePayload)
	if err != nil {
		_, _ = s.client.Revoke(ctx, lease.ID)
		return nil, fmt.Errorf("scheduler: marshal lease payload: %w", err)
	}

	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(string(kv.Key)), "=", kv.ModRevision),
		clientv3.Compare(clientv3.ModRevision(jobKey), "=", jobResp.Kvs[0].ModRevision),
	).Then(
		clientv3.OpDelete(string(kv.Key)),
		clientv3.OpPut(jobKey, string(jobBytes)),
		clientv3.OpPut(s.leaseKey(record.ID), string(leaseBytes), clientv3.WithLease(lease.ID)),
	)

	txnResp, err := txn.Commit()
	if err != nil {
		_, _ = s.client.Revoke(ctx, lease.ID)
		return nil, fmt.Errorf("scheduler: claim commit: %w", err)
	}
	if !txnResp.Succeeded {
		_, _ = s.client.Revoke(ctx, lease.ID)
		return nil, errRetryClaim
	}

	s.metrics.QueueDequeued(entry.Priority)
	s.metrics.ObserveClaimLatency(entry.Priority, latency)

	return &ClaimResult{
		NodeID:  req.NodeID,
		LeaseID: lease.ID,
		Job:     record.toJob(),
	}, nil
}

func (s *Scheduler) jobKey(ticket, jobID string) string {
	return path.Join(s.jobsPrefix, ticket, "jobs", jobID)
}

func (s *Scheduler) queueKey(priority, jobID string) string {
	return path.Join(s.queuePrefix, priority, jobID)
}

func (s *Scheduler) leaseKey(jobID string) string {
	return path.Join(s.leasesPrefix, jobID)
}

func (s *Scheduler) gcKey(jobID string) string {
	return path.Join(s.gcPrefix, jobID)
}

func (s *Scheduler) lookupJobKey(ctx context.Context, jobID string) (string, string, error) {
	resp, err := s.client.Get(ctx, s.jobsPrefix, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		return "", "", fmt.Errorf("scheduler: lookup job key: %w", err)
	}
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		if strings.HasSuffix(key, "/"+jobID) {
			segments := strings.Split(key, "/")
			if len(segments) < 3 {
				continue
			}
			ticket := segments[len(segments)-3]
			return key, ticket, nil
		}
	}
	return "", "", fmt.Errorf("scheduler: job %s not found", jobID)
}

type compiledOptions struct {
	jobsPrefix   string
	queuePrefix  string
	leasesPrefix string
	nodesPrefix  string
	gcPrefix     string
	leaseTTL     time.Duration
	clockSkew    time.Duration
	now          func() time.Time
	idGenerator  func() string
	metrics      metricsx.SchedulerRecorder
}

func compileOptions(opts Options) compiledOptions {
	cfg := compiledOptions{
		jobsPrefix:   normalizePrefix(opts.JobsPrefix, "mods"),
		queuePrefix:  normalizePrefix(opts.QueuePrefix, "queue/mods"),
		leasesPrefix: normalizePrefix(opts.LeasesPrefix, "leases/jobs"),
		nodesPrefix:  normalizePrefix(opts.NodesPrefix, "nodes"),
		gcPrefix:     normalizePrefix(opts.GCPrefix, "gc/jobs"),
		leaseTTL:     opts.LeaseTTL,
		clockSkew:    opts.ClockSkewBuffer,
		now:          opts.Now,
		idGenerator:  opts.IDGenerator,
		metrics:      opts.Metrics,
	}

	if cfg.leaseTTL <= 0 {
		cfg.leaseTTL = 120 * time.Second
	}
	if cfg.clockSkew < 0 {
		cfg.clockSkew = 0
	}
	if cfg.now == nil {
		cfg.now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.idGenerator == nil {
		cfg.idGenerator = func() string { return uuid.NewString() }
	}
	if cfg.metrics == nil {
		cfg.metrics = metricsx.NewNoopSchedulerRecorder()
	}

	return cfg
}

func normalizePrefix(prefix, fallback string) string {
	p := strings.Trim(prefix, "/")
	if p == "" {
		p = fallback
	}
	return p
}

func normalizeShiftResult(raw string, state JobState) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "unknown":
		switch state {
		case JobStateSucceeded:
			return ShiftResultPassed
		case JobStateFailed, JobStateInspectionReady:
			return ShiftResultFailed
		default:
			return ""
		}
	case "passed", "pass", "success", "succeeded":
		return ShiftResultPassed
	case "failed", "fail", "failure":
		return ShiftResultFailed
	default:
		return value
	}
}

type jobRecord struct {
	ID             string                  `json:"id"`
	Ticket         string                  `json:"ticket"`
	StepID         string                  `json:"step_id"`
	Priority       string                  `json:"priority"`
	State          JobState                `json:"state"`
	CreatedAt      string                  `json:"created_at"`
	EnqueuedAt     string                  `json:"enqueued_at"`
	ClaimedAt      string                  `json:"claimed_at,omitempty"`
	CompletedAt    string                  `json:"completed_at,omitempty"`
	ExpiresAt      string                  `json:"expires_at,omitempty"`
	LeaseID        int64                   `json:"lease_id,omitempty"`
	LeaseExpiresAt string                  `json:"lease_expires_at,omitempty"`
	ClaimedBy      string                  `json:"claimed_by,omitempty"`
	RetryAttempt   int                     `json:"retry_attempt"`
	MaxAttempts    int                     `json:"max_attempts"`
	Metadata       map[string]string       `json:"metadata,omitempty"`
	Artifacts      map[string]string       `json:"artifacts,omitempty"`
	Bundles        map[string]bundleRecord `json:"bundles,omitempty"`
	Shift          *shiftRecord            `json:"shift,omitempty"`
	Retention      *retentionRecord        `json:"retention,omitempty"`
	NodeSnapshot   *nodeSnapshotRecord     `json:"node_snapshot,omitempty"`
	Error          *JobError               `json:"error,omitempty"`
}

func (r jobRecord) toJob() *Job {
	return &Job{
		ID:             r.ID,
		Ticket:         r.Ticket,
		StepID:         r.StepID,
		Priority:       r.Priority,
		State:          r.State,
		CreatedAt:      decodeTime(r.CreatedAt),
		EnqueuedAt:     decodeTime(r.EnqueuedAt),
		ClaimedAt:      decodeTime(r.ClaimedAt),
		CompletedAt:    decodeTime(r.CompletedAt),
		ExpiresAt:      decodeTime(r.ExpiresAt),
		LeaseID:        clientv3.LeaseID(r.LeaseID),
		LeaseExpiresAt: decodeTime(r.LeaseExpiresAt),
		ClaimedBy:      r.ClaimedBy,
		RetryAttempt:   r.RetryAttempt,
		MaxAttempts:    r.MaxAttempts,
		Metadata:       cloneMap(r.Metadata),
		Artifacts:      cloneMap(r.Artifacts),
		Bundles:        exportBundleRecords(r.Bundles),
		Shift:          exportShiftSummary(r.Shift),
		Retention:      exportRetention(r.Retention),
		NodeSnapshot:   exportNodeSnapshot(r.NodeSnapshot),
		Error:          r.Error,
	}
}

func decodeJobRecord(data []byte) (jobRecord, error) {
	var record jobRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return jobRecord{}, err
	}
	return record, nil
}

type bundleRecord struct {
	CID       string `json:"cid,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Retained  bool   `json:"retained,omitempty"`
	TTL       string `json:"ttl,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type retentionRecord struct {
	Retained   bool   `json:"retained,omitempty"`
	TTL        string `json:"ttl,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Bundle     string `json:"bundle,omitempty"`
	BundleCID  string `json:"bundle_cid,omitempty"`
	Inspection bool   `json:"inspection,omitempty"`
}

type nodeSnapshotRecord struct {
	NodeID     string         `json:"node_id"`
	Capacity   map[string]any `json:"capacity,omitempty"`
	CapacityAt string         `json:"capacity_at,omitempty"`
	Status     map[string]any `json:"status,omitempty"`
	StatusAt   string         `json:"status_at,omitempty"`
}

// exportBundleRecords converts stored bundle metadata into public scheduler state.
func exportBundleRecords(src map[string]bundleRecord) map[string]BundleRecord {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]BundleRecord, len(src))
	for name, rec := range src {
		dst[name] = BundleRecord(rec)
	}
	return dst
}

func exportShiftSummary(rec *shiftRecord) *ShiftSummary {
	if rec == nil {
		return nil
	}
	summary := &ShiftSummary{Result: rec.Result}
	if rec.DurationSeconds > 0 {
		summary.Duration = time.Duration(rec.DurationSeconds * float64(time.Second))
	}
	return summary
}

func exportRetention(src *retentionRecord) *JobRetention {
	if src == nil {
		return nil
	}
	return &JobRetention{
		Retained:   src.Retained,
		TTL:        src.TTL,
		ExpiresAt:  src.ExpiresAt,
		Bundle:     src.Bundle,
		BundleCID:  src.BundleCID,
		Inspection: src.Inspection,
	}
}

func exportNodeSnapshot(rec *nodeSnapshotRecord) *JobNodeSnapshot {
	if rec == nil {
		return nil
	}
	snapshot := &JobNodeSnapshot{
		NodeID: rec.NodeID,
	}
	if len(rec.Capacity) > 0 {
		snapshot.Capacity = cloneAnyMap(rec.Capacity)
	}
	if len(rec.Status) > 0 {
		snapshot.Status = cloneAnyMap(rec.Status)
	}
	if strings.TrimSpace(rec.CapacityAt) != "" {
		snapshot.CapacityAt = decodeTime(rec.CapacityAt)
	}
	if strings.TrimSpace(rec.StatusAt) != "" {
		snapshot.StatusAt = decodeTime(rec.StatusAt)
	}
	return snapshot
}

// normalizeBundleRecords trims bundle fields and computes expiry timestamps.
func normalizeBundleRecords(src map[string]BundleRecord, completedAt time.Time) map[string]bundleRecord {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]bundleRecord, len(src))
	for name, rec := range src {
		cid := strings.TrimSpace(rec.CID)
		digest := strings.TrimSpace(rec.Digest)
		ttl := strings.TrimSpace(rec.TTL)
		expires := strings.TrimSpace(rec.ExpiresAt)
		if expires == "" && !completedAt.IsZero() && ttl != "" {
			if duration, err := time.ParseDuration(ttl); err == nil && duration > 0 {
				expires = completedAt.Add(duration).UTC().Format(time.RFC3339Nano)
			}
		}
		dst[name] = bundleRecord{
			CID:       cid,
			Digest:    digest,
			Size:      rec.Size,
			Retained:  rec.Retained,
			TTL:       ttl,
			ExpiresAt: expires,
		}
	}
	return dst
}

func computeRetentionExpiry(bundles map[string]bundleRecord, completedAt time.Time) time.Time {
	var latest time.Time
	for _, rec := range bundles {
		if expires := decodeTime(rec.ExpiresAt); !expires.IsZero() {
			if expires.After(latest) {
				latest = expires
			}
			continue
		}
		ttl := strings.TrimSpace(rec.TTL)
		if ttl == "" {
			continue
		}
		duration, err := time.ParseDuration(ttl)
		if err != nil || duration <= 0 {
			continue
		}
		candidate := completedAt.Add(duration).UTC()
		if candidate.After(latest) {
			latest = candidate
		}
	}
	return latest
}

func deriveRetentionRecord(bundles map[string]bundleRecord, expiry time.Time, inspection bool) *retentionRecord {
	var (
		found bool
		name  string
		rec   bundleRecord
	)
	if len(bundles) > 0 {
		if candidate, ok := bundles["logs"]; ok && (candidate.Retained || strings.TrimSpace(candidate.TTL) != "" || strings.TrimSpace(candidate.ExpiresAt) != "" || strings.TrimSpace(candidate.CID) != "") {
			found = true
			name = "logs"
			rec = candidate
		} else {
			for key, candidate := range bundles {
				if candidate.Retained || strings.TrimSpace(candidate.TTL) != "" || strings.TrimSpace(candidate.ExpiresAt) != "" || strings.TrimSpace(candidate.CID) != "" {
					found = true
					name = key
					rec = candidate
					break
				}
			}
		}
	}

	if !found && !inspection {
		return nil
	}

	summary := &retentionRecord{
		Bundle:     name,
		BundleCID:  strings.TrimSpace(rec.CID),
		TTL:        strings.TrimSpace(rec.TTL),
		Retained:   rec.Retained,
		Inspection: inspection,
	}

	if expiry.IsZero() {
		summary.ExpiresAt = ""
	} else {
		summary.ExpiresAt = encodeTime(expiry)
	}
	if trimmed := strings.TrimSpace(rec.ExpiresAt); trimmed != "" {
		summary.ExpiresAt = trimmed
	}
	if summary.Retained || inspection {
		summary.Retained = true
	}
	if !found {
		summary.BundleCID = ""
		summary.TTL = ""
		summary.Bundle = ""
	}
	return summary
}

type queueEntry struct {
	JobID        string            `json:"job_id"`
	Ticket       string            `json:"ticket"`
	StepID       string            `json:"step_id"`
	Priority     string            `json:"priority"`
	RetryAttempt int               `json:"retry_attempt"`
	MaxAttempts  int               `json:"max_attempts"`
	EnqueuedAt   string            `json:"enqueued_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type shiftRecord struct {
	Result          string  `json:"result,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
}

type leaseEntry struct {
	JobID    string `json:"job_id"`
	Ticket   string `json:"ticket"`
	Priority string `json:"priority"`
}

type gcEntry struct {
	JobID      string   `json:"job_id"`
	Ticket     string   `json:"ticket"`
	State      JobState `json:"state"`
	FinalState string   `json:"final_state"`
	ExpiresAt  string   `json:"expires_at"`
}

func encodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func decodeTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func cloneMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func decodeKVMap(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil
	}
	if len(generic) == 0 {
		return nil
	}
	for key, value := range generic {
		if str, ok := value.(string); ok {
			generic[key] = strings.TrimSpace(str)
		}
	}
	return generic
}

func snapshotTimestamp(fields map[string]any, fallback time.Time) string {
	if len(fields) == 0 {
		return ""
	}
	for _, key := range []string{"heartbeat", "checked_at", "observed_at", "timestamp"} {
		if raw, ok := fields[key]; ok {
			if str, ok := raw.(string); ok {
				trimmed := strings.TrimSpace(str)
				if trimmed != "" {
					return trimmed
				}
			}
		}
	}
	if fallback.IsZero() {
		return ""
	}
	return encodeTime(fallback)
}

var errRetryClaim = errors.New("scheduler: retry claim")
