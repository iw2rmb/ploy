package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	HydrationSnapshotBundleKey = "hydration_snapshot"
	HydrationSnapshotCIDKey    = "hydration_snapshot_cid"
	HydrationSnapshotDigestKey = "hydration_snapshot_digest"
	HydrationSnapshotSizeKey   = "hydration_snapshot_size"
	HydrationSnapshotTTL       = "24h"
)

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
	bundleInputs := cloneBundleMap(req.Bundles)
	if rec, ok := hydrationBundleFromArtifacts(req.Artifacts); ok {
		if bundleInputs == nil {
			bundleInputs = make(map[string]BundleRecord, len(req.Bundles)+1)
		}
		bundleInputs[HydrationSnapshotBundleKey] = rec
	}
	if len(bundleInputs) > 0 {
		record.Bundles = normalizeBundleRecords(bundleInputs, completedAt)
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

    gateDuration := time.Duration(0)
    gateResult := ""
    if req.Gate != nil {
        gateDuration = req.Gate.Duration
        if gateDuration < 0 {
            gateDuration = 0
        }
        gateResult = normalizeGateResult(req.Gate.Result, record.State)
        if gateDuration > 0 || gateResult != "" {
            record.Gate = &gateRecord{
                Result:          gateResult,
                DurationSeconds: gateDuration.Seconds(),
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
    if gateDuration > 0 {
        s.metrics.ObserveGateDuration(record.StepID, gateResult, gateDuration)
    }

	if record.LeaseID != 0 {
		if _, err := s.client.Revoke(ctx, clientv3.LeaseID(record.LeaseID)); err != nil {
			log.Printf("scheduler: revoke lease %d: %v", record.LeaseID, err)
		}
	}

	return record.toJob(), nil
}

func hydrationBundleFromArtifacts(artifacts map[string]string) (BundleRecord, bool) {
	cid := strings.TrimSpace(artifacts[HydrationSnapshotCIDKey])
	if cid == "" {
		return BundleRecord{}, false
	}
	digest := strings.TrimSpace(artifacts[HydrationSnapshotDigestKey])
	sizeStr := strings.TrimSpace(artifacts[HydrationSnapshotSizeKey])
	sizeValue := int64(0)
	if sizeStr != "" {
		if parsed, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
			sizeValue = parsed
		}
	}
	return BundleRecord{
		CID:      cid,
		Digest:   digest,
		Size:     sizeValue,
		Retained: true,
		TTL:      HydrationSnapshotTTL,
	}, true
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

func normalizeGateResult(raw string, state JobState) string {
    value := strings.ToLower(strings.TrimSpace(raw))
    switch value {
    case "", "unknown":
        switch state {
        case JobStateSucceeded:
            return GateResultPassed
        case JobStateFailed, JobStateInspectionReady:
            return GateResultFailed
        default:
            return ""
        }
    case "passed", "pass", "success", "succeeded":
        return GateResultPassed
    case "failed", "fail", "failure":
        return GateResultFailed
    default:
        return value
    }
}

var errRetryClaim = errors.New("scheduler: retry claim")
